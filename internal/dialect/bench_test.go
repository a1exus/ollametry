package dialect

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// benchFixtures are recorded responses, one per format and API family.
var benchFixtures = []struct {
	file   string
	format Format
	kind   Kind
}{
	{"native_chat_stream.ndjson", FormatNDJSON, KindNative},
	{"native_chat.json", FormatJSON, KindNative},
	{"openai_chat_stream_usage.sse", FormatSSE, KindOpenAI},
	{"anthropic_messages_stream.sse", FormatSSE, KindAnthropic},
	{"responses_stream.sse", FormatSSE, KindResponses},
}

func readBenchFixture(b *testing.B, name string) []byte {
	b.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		b.Fatalf("read fixture %s: %v", name, err)
	}
	return body
}

// chunks splits body the way it arrives from Ollama: one write per line.
func chunks(body []byte) [][]byte {
	return bytes.SplitAfter(body, []byte("\n"))
}

// BenchmarkFramer measures splitting a response into documents.
func BenchmarkFramer(b *testing.B) {
	for _, fx := range benchFixtures {
		body := readBenchFixture(b, fx.file)
		parts := chunks(body)
		b.Run(fx.file, func(b *testing.B) {
			b.SetBytes(int64(len(body)))
			b.ReportAllocs()
			for b.Loop() {
				f := NewFramer(fx.format, 32<<20, func([]byte) {})
				for _, p := range parts {
					f.Write(p)
				}
				f.Close()
			}
		})
	}
}

// BenchmarkDecode measures framing and decoding a whole response: the work
// ollametry adds per inference request on top of relaying it.
func BenchmarkDecode(b *testing.B) {
	at := time.Unix(1_700_000_000, 0)
	for _, fx := range benchFixtures {
		body := readBenchFixture(b, fx.file)
		parts := chunks(body)
		b.Run(fx.file, func(b *testing.B) {
			b.SetBytes(int64(len(body)))
			b.ReportAllocs()
			for b.Loop() {
				var r Result
				dec := NewDecoder(fx.kind)
				f := NewFramer(fx.format, 32<<20, func(doc []byte) { dec.Decode(doc, at, &r) })
				for _, p := range parts {
					f.Write(p)
				}
				f.Close()
			}
		})
	}
}
