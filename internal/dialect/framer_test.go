package dialect

import (
	"slices"
	"testing"
)

func frame(format Format, maxDoc int, chunks ...string) (docs []string, dropped int) {
	f := NewFramer(format, maxDoc, func(doc []byte) {
		docs = append(docs, string(doc))
	})
	for _, c := range chunks {
		f.Write([]byte(c))
	}
	f.Close()
	return docs, f.Dropped()
}

func TestFramer(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		format      Format
		maxDoc      int
		chunks      []string
		want        []string
		wantDropped int
	}{
		{
			name: "ndjson split across chunks", format: FormatNDJSON, maxDoc: 64,
			chunks: []string{`{"a":`, "1}\n{\"b\"", ":2}\n"},
			want:   []string{`{"a":1}`, `{"b":2}`},
		},
		{
			name: "ndjson last line without newline", format: FormatNDJSON, maxDoc: 64,
			chunks: []string{"{\"a\":1}\n{\"b\":2}"},
			want:   []string{`{"a":1}`, `{"b":2}`},
		},
		{
			name: "ndjson crlf and blank lines", format: FormatNDJSON, maxDoc: 64,
			chunks: []string{"{\"a\":1}\r\n\r\n{\"b\":2}\r\n"},
			want:   []string{`{"a":1}`, `{"b":2}`},
		},
		{
			name: "ndjson oversized line dropped, neighbours kept", format: FormatNDJSON, maxDoc: 8,
			chunks: []string{"{\"a\":1}\n{\"big\":\"xxxx", "xxxxxxxx\"}\n{\"b\":2}\n"},
			want:   []string{`{"a":1}`, `{"b":2}`}, wantDropped: 1,
		},
		{
			name: "json whole body", format: FormatJSON, maxDoc: 64,
			chunks: []string{"{\n  \"a\": ", "1\n}\n"},
			want:   []string{"{\n  \"a\": 1\n}"},
		},
		{
			name: "json oversized body", format: FormatJSON, maxDoc: 4,
			chunks: []string{`{"a":1}`},
			want:   nil, wantDropped: 1,
		},
		{
			name: "sse events with event lines, comments and done", format: FormatSSE, maxDoc: 64,
			chunks: []string{": ping\n\nevent: x\ndata: {\"a\":1}\n\n", "data: [DONE]\n\n"},
			want:   []string{`{"a":1}`, "[DONE]"},
		},
		{
			name: "sse multi-line data joined with newline", format: FormatSSE, maxDoc: 64,
			chunks: []string{"data: {\"a\":\ndata: 1}\n\n"},
			want:   []string{"{\"a\":\n1}"},
		},
		{
			name: "sse no space after colon and crlf", format: FormatSSE, maxDoc: 64,
			chunks: []string{"data:{\"a\":1}\r\n\r\n"},
			want:   []string{`{"a":1}`},
		},
		{
			name: "sse unterminated final event discarded", format: FormatSSE, maxDoc: 64,
			chunks: []string{"data: {\"a\":1}\n\ndata: {\"b\":2}\n"},
			want:   []string{`{"a":1}`},
		},
		{
			name: "sse oversized event dropped, next kept", format: FormatSSE, maxDoc: 16,
			chunks: []string{"data: {\"big\":\"xxxxxxxxxxxxxxxx\"}\ndata: more\n\ndata: {\"b\":2}\n\n"},
			want:   []string{`{"b":2}`}, wantDropped: 1,
		},
		{
			name: "unknown format ignored", format: FormatUnknown, maxDoc: 64,
			chunks: []string{"{\"a\":1}\n"},
			want:   nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, dropped := frame(tt.format, tt.maxDoc, tt.chunks...)
			if !slices.Equal(got, tt.want) {
				t.Errorf("docs = %q, want %q", got, tt.want)
			}
			if dropped != tt.wantDropped {
				t.Errorf("dropped = %d, want %d", dropped, tt.wantDropped)
			}
		})
	}
}

func TestFormatFromContentType(t *testing.T) {
	t.Parallel()
	tests := []struct {
		contentType string
		want        Format
	}{
		{"application/json; charset=utf-8", FormatJSON},
		{"application/x-ndjson", FormatNDJSON},
		{"text/event-stream", FormatSSE},
		{"TEXT/EVENT-STREAM; charset=utf-8", FormatSSE},
		{"text/plain", FormatUnknown},
		{"", FormatUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.contentType, func(t *testing.T) {
			t.Parallel()
			if got := FormatFromContentType(tt.contentType); got != tt.want {
				t.Errorf("FormatFromContentType(%q) = %v, want %v", tt.contentType, got, tt.want)
			}
		})
	}
}
