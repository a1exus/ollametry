package dialect

import (
	"bytes"
	"mime"
)

// Format is how an upstream response body frames its JSON documents.
type Format int

const (
	// FormatUnknown bodies are relayed but not inspected.
	FormatUnknown Format = iota
	// FormatJSON is a single document: the whole body.
	FormatJSON
	// FormatNDJSON is one document per line (Ollama's native streaming).
	FormatNDJSON
	// FormatSSE is Server-Sent Events with documents in the data field
	// (OpenAI- and Anthropic-compatible streaming).
	FormatSSE
)

// FormatFromContentType maps a Content-Type header value to a Format.
func FormatFromContentType(contentType string) Format {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return FormatUnknown
	}
	switch mediaType {
	case "application/json":
		return FormatJSON
	case "application/x-ndjson", "application/ndjson", "application/jsonl":
		return FormatNDJSON
	case "text/event-stream":
		return FormatSSE
	default:
		return FormatUnknown
	}
}

// Streaming reports whether the format delivers documents incrementally.
func (f Format) Streaming() bool {
	return f == FormatNDJSON || f == FormatSSE
}

// Framer splits a response body, fed in arbitrary chunks, into documents.
//
// It never fails: a document larger than the limit is dropped, and everything
// after it is still framed, so a single oversized record (an embedding vector,
// say) does not cost the metrics carried by the records around it.
type Framer struct {
	format    Format
	maxDoc    int
	emit      func(doc []byte)
	line      []byte
	data      []byte
	overflow  bool
	skipEvent bool
	dropped   int
}

// NewFramer returns a Framer that calls emit for every complete document.
// emit must not retain the slice it is given.
func NewFramer(format Format, maxDoc int, emit func(doc []byte)) *Framer {
	return &Framer{format: format, maxDoc: maxDoc, emit: emit}
}

// Dropped returns how many documents exceeded the size limit.
func (f *Framer) Dropped() int {
	return f.dropped
}

// Write consumes the next chunk of the body.
func (f *Framer) Write(chunk []byte) {
	switch f.format {
	case FormatJSON:
		f.appendBounded(chunk)
	case FormatNDJSON, FormatSSE:
		for len(chunk) > 0 {
			i := bytes.IndexByte(chunk, '\n')
			if i < 0 {
				f.appendBounded(chunk)
				return
			}
			f.appendBounded(chunk[:i])
			f.endLine()
			chunk = chunk[i+1:]
		}
	case FormatUnknown:
	}
}

// Close flushes what is left once the body has ended.
func (f *Framer) Close() {
	switch f.format {
	case FormatJSON, FormatNDJSON:
		f.endLine()
	case FormatSSE, FormatUnknown:
		// Per the SSE spec, an event not terminated by a blank line is
		// discarded at end of stream.
	}
}

func (f *Framer) appendBounded(p []byte) {
	if f.overflow {
		return
	}
	if len(f.line)+len(p) > f.maxDoc {
		f.overflow = true
		f.line = f.line[:0]
		return
	}
	f.line = append(f.line, p...)
}

func (f *Framer) endLine() {
	line := bytes.TrimSuffix(f.line, []byte{'\r'})
	overflow := f.overflow
	f.line, f.overflow = f.line[:0], false

	if f.format == FormatSSE {
		f.sseLine(line, overflow)
		return
	}
	if overflow {
		f.dropped++
		return
	}
	if line = bytes.TrimSpace(line); len(line) > 0 {
		f.emit(line)
	}
}

// sseLine applies one line of the event-stream grammar.
func (f *Framer) sseLine(line []byte, overflow bool) {
	if overflow {
		// The oversized line was a field of the current event; drop the event.
		f.data = f.data[:0]
		f.dropped++
		f.skipEvent = true
		return
	}
	if len(line) == 0 {
		if !f.skipEvent && len(f.data) > 0 {
			f.emit(bytes.TrimSuffix(f.data, []byte{'\n'}))
		}
		f.data, f.skipEvent = f.data[:0], false
		return
	}
	if line[0] == ':' || f.skipEvent {
		return
	}
	field, value, found := bytes.Cut(line, []byte{':'})
	if !bytes.Equal(field, []byte("data")) {
		return
	}
	if found {
		value = bytes.TrimPrefix(value, []byte{' '})
	}
	if len(f.data)+len(value)+1 > f.maxDoc {
		f.data = f.data[:0]
		f.dropped++
		f.skipEvent = true
		return
	}
	f.data = append(f.data, value...)
	f.data = append(f.data, '\n')
}
