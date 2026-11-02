package dialect

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// seedFixtures adds every recorded response to the corpus with the format and
// decoder kind it belongs to.
func seedFixtures(f *testing.F, add func(body []byte, format Format, kind Kind)) {
	f.Helper()
	files, err := filepath.Glob(filepath.Join("testdata", "*"))
	if err != nil {
		f.Fatalf("list fixtures: %v", err)
	}
	for _, name := range files {
		body, err := os.ReadFile(name)
		if err != nil {
			f.Fatalf("read fixture %s: %v", name, err)
		}
		add(body, fixtureFormat(name), fixtureKind(filepath.Base(name)))
	}
}

func fixtureFormat(name string) Format {
	switch filepath.Ext(name) {
	case ".ndjson":
		return FormatNDJSON
	case ".sse":
		return FormatSSE
	}
	return FormatJSON
}

func fixtureKind(base string) Kind {
	for prefix, kind := range map[string]Kind{
		"native_": KindNative, "openai_": KindOpenAI,
		"anthropic_": KindAnthropic, "responses_": KindResponses,
	} {
		if len(base) >= len(prefix) && base[:len(prefix)] == prefix {
			return kind
		}
	}
	return KindNone
}

// FuzzFramer checks that framing does not depend on how the body is chunked
// and that no emitted document exceeds the size limit.
func FuzzFramer(f *testing.F) {
	seedFixtures(f, func(body []byte, format Format, _ Kind) {
		f.Add(body, uint8(format), uint16(4096), uint16(len(body)/2))
		f.Add(body, uint8(format), uint16(64), uint16(1))
	})
	f.Add([]byte("data: a\n\ndata: b"), uint8(FormatSSE), uint16(8), uint16(3))
	f.Add([]byte("{\"a\":1}\r\n\r\n{\"b\""), uint8(FormatNDJSON), uint16(8), uint16(0))

	f.Fuzz(func(t *testing.T, body []byte, format uint8, maxDoc, split uint16) {
		fm := Format(format % uint8(FormatSSE+1))
		limit := int(maxDoc%8192) + 1

		whole, wholeDropped := frameChunks(fm, limit, body)
		for _, d := range whole {
			if len(d) > limit {
				t.Fatalf("emitted %d bytes, limit %d", len(d), limit)
			}
		}
		if fm == FormatJSON && len(whole) > 1 {
			t.Fatalf("JSON body framed as %d documents", len(whole))
		}

		cut := int(split) % (len(body) + 1)
		parts, partsDropped := frameChunks(fm, limit, body[:cut], body[cut:])
		if !slices.Equal(whole, parts) || wholeDropped != partsDropped {
			t.Fatalf("split at %d changed the result:\nwhole %q dropped %d\nsplit %q dropped %d",
				cut, whole, wholeDropped, parts, partsDropped)
		}

		bytewise := make([][]byte, len(body))
		for i := range body {
			bytewise[i] = body[i : i+1]
		}
		single, singleDropped := frameChunks(fm, limit, bytewise...)
		if !slices.Equal(whole, single) || wholeDropped != singleDropped {
			t.Fatalf("byte-at-a-time framing changed the result:\nwhole %q dropped %d\nbytes %q dropped %d",
				whole, wholeDropped, single, singleDropped)
		}
	})
}

func frameChunks(format Format, maxDoc int, chunks ...[]byte) (docs []string, dropped int) {
	fr := NewFramer(format, maxDoc, func(doc []byte) {
		docs = append(docs, string(doc))
	})
	for _, c := range chunks {
		fr.Write(c)
	}
	fr.Close()
	return docs, fr.Dropped()
}

// FuzzDecoders checks that arbitrary documents never panic a decoder and that
// whatever ends up in a metric attribute stays bounded.
func FuzzDecoders(f *testing.F) {
	seedFixtures(f, func(body []byte, format Format, kind Kind) {
		if kind != KindNone {
			f.Add(body, uint8(format), uint8(kind))
		}
	})
	f.Add([]byte(`{"error":{"type":"Not A Valid Type"}}`), uint8(FormatJSON), uint8(KindOpenAI))
	f.Add([]byte(`{"error":42}`), uint8(FormatJSON), uint8(KindNative))

	f.Fuzz(func(t *testing.T, body []byte, format, kind uint8) {
		k := Kind(kind%uint8(KindResponses)) + KindNative
		fm := Format(format % uint8(FormatSSE+1))
		dec := NewDecoder(k)

		var r Result
		start := time.Unix(1_700_000_000, 0)
		at := start
		fr := NewFramer(fm, 1<<16, func(doc []byte) {
			at = at.Add(time.Millisecond)
			dec.Decode(doc, at, &r)
		})
		fr.Write(body)
		fr.Close()

		if r.ErrorType != "" && r.ErrorType != ErrorTypeStream && !errorTypePattern.MatchString(r.ErrorType) {
			t.Fatalf("unbounded error.type %q", r.ErrorType)
		}
		if r.FirstTokenAt.IsZero() != r.LastTokenAt.IsZero() {
			t.Fatalf("token timestamps half set: first %v last %v", r.FirstTokenAt, r.LastTokenAt)
		}
		if !r.FirstTokenAt.IsZero() && (r.FirstTokenAt.Before(start) || r.LastTokenAt.Before(r.FirstTokenAt) || r.LastTokenAt.After(at)) {
			t.Fatalf("token timestamps out of order: start %v first %v last %v end %v",
				start, r.FirstTokenAt, r.LastTokenAt, at)
		}
	})
}
