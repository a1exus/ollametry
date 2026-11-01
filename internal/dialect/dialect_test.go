package dialect

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// replay frames body in chunks of chunkSize bytes and decodes every document,
// advancing a fake clock by one millisecond per document.
func replay(t *testing.T, kind Kind, format Format, body []byte, chunkSize int) Result {
	t.Helper()
	var (
		r   Result
		dec = NewDecoder(kind)
		now = time.Unix(0, 0)
	)
	framer := NewFramer(format, 1<<20, func(doc []byte) {
		now = now.Add(time.Millisecond)
		dec.Decode(doc, now, &r)
	})
	for len(body) > 0 {
		n := min(chunkSize, len(body))
		framer.Write(body[:n])
		body = body[n:]
	}
	framer.Close()
	return r
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return b
}

// want is the subset of Result each fixture is checked against.
type want struct {
	model          string
	tokensReported bool
	input, cached  int64
	output         int64
	firstToken     bool
	evalDuration   time.Duration
	finishReason   string
	complete       bool
	errorType      string
}

func TestDecodersAgainstRecordedResponses(t *testing.T) {
	t.Parallel()
	const model = "granite4.2:8b"
	tests := []struct {
		file   string
		kind   Kind
		format Format
		want   want
	}{
		{
			file: "native_generate_stream.ndjson", kind: KindNative, format: FormatNDJSON,
			want: want{model: model, tokensReported: true, input: 21, cached: 20, output: 12, firstToken: true,
				evalDuration: 773633000, finishReason: "length", complete: true},
		},
		{
			file: "native_chat_stream.ndjson", kind: KindNative, format: FormatNDJSON,
			want: want{model: model, tokensReported: true, input: 21, cached: 20, output: 12, firstToken: true,
				evalDuration: 820472000, finishReason: "length", complete: true},
		},
		{
			file: "native_generate.json", kind: KindNative, format: FormatJSON,
			want: want{model: model, tokensReported: true, input: 21, cached: 20, output: 12, firstToken: true,
				evalDuration: 803178000, finishReason: "length", complete: true},
		},
		{
			file: "native_chat.json", kind: KindNative, format: FormatJSON,
			want: want{model: model, tokensReported: true, input: 21, cached: 20, output: 12, firstToken: true,
				evalDuration: 828543000, finishReason: "length", complete: true},
		},
		{
			file: "native_error_model.json", kind: KindNative, format: FormatJSON,
			want: want{errorType: ErrorTypeStream},
		},
		{
			file: "native_embed.json", kind: KindNative, format: FormatJSON,
			want: want{errorType: ErrorTypeStream},
		},
		{
			// Without stream_options.include_usage the stream has no counts.
			file: "openai_chat_stream.sse", kind: KindOpenAI, format: FormatSSE,
			want: want{model: model, firstToken: true, finishReason: "length", complete: true},
		},
		{
			file: "openai_chat_stream_usage.sse", kind: KindOpenAI, format: FormatSSE,
			want: want{model: model, tokensReported: true, input: 21, cached: 20, output: 12, firstToken: true,
				evalDuration: 816 * time.Millisecond, finishReason: "length", complete: true},
		},
		{
			file: "openai_completion_stream.sse", kind: KindOpenAI, format: FormatSSE,
			want: want{model: model, tokensReported: true, input: 21, cached: 20, output: 12, firstToken: true,
				evalDuration: 810 * time.Millisecond, finishReason: "length", complete: true},
		},
		{
			file: "openai_chat.json", kind: KindOpenAI, format: FormatJSON,
			want: want{model: model, tokensReported: true, input: 21, cached: 20, output: 12, finishReason: "length"},
		},
		{
			file: "openai_error_model.json", kind: KindOpenAI, format: FormatJSON,
			want: want{errorType: "not_found_error"},
		},
		{
			file: "openai_embeddings.json", kind: KindOpenAI, format: FormatJSON,
			want: want{errorType: "api_error"},
		},
		{
			// message_start reports 6 input tokens; the final message_delta
			// corrects it to 1 uncached + 20 cached.
			file: "anthropic_messages_stream.sse", kind: KindAnthropic, format: FormatSSE,
			want: want{model: model, tokensReported: true, input: 21, cached: 20, output: 12, firstToken: true,
				finishReason: "max_tokens", complete: true},
		},
		{
			file: "anthropic_messages.json", kind: KindAnthropic, format: FormatJSON,
			want: want{model: model, tokensReported: true, input: 21, cached: 20, output: 12,
				finishReason: "max_tokens", complete: true},
		},
		{
			file: "anthropic_error_model.json", kind: KindAnthropic, format: FormatJSON,
			want: want{errorType: "not_found_error"},
		},
		{
			file: "responses_stream.sse", kind: KindResponses, format: FormatSSE,
			want: want{model: model, tokensReported: true, input: 21, cached: 20, output: 12, firstToken: true,
				finishReason: "completed", complete: true},
		},
		{
			file: "responses.json", kind: KindResponses, format: FormatJSON,
			want: want{model: model, tokensReported: true, input: 21, cached: 20, output: 12,
				finishReason: "completed", complete: true},
		},
	}
	for _, tt := range tests {
		body := fixture(t, tt.file)
		// Chunk sizes cover byte-by-byte delivery, splits inside documents and
		// the whole body at once.
		for _, chunk := range []int{1, 7, len(body)} {
			t.Run(tt.file, func(t *testing.T) {
				t.Parallel()
				got := replay(t, tt.kind, tt.format, body, chunk)
				assertResult(t, got, tt.want)
			})
		}
	}
}

func assertResult(t *testing.T, got Result, w want) {
	t.Helper()
	if got.ResponseModel != w.model {
		t.Errorf("ResponseModel = %q, want %q", got.ResponseModel, w.model)
	}
	if got.TokensReported != w.tokensReported {
		t.Errorf("TokensReported = %v, want %v", got.TokensReported, w.tokensReported)
	}
	if got.InputTokens != w.input || got.CachedInputTokens != w.cached || got.OutputTokens != w.output {
		t.Errorf("tokens in/cached/out = %d/%d/%d, want %d/%d/%d",
			got.InputTokens, got.CachedInputTokens, got.OutputTokens, w.input, w.cached, w.output)
	}
	if hasFirst := !got.FirstTokenAt.IsZero(); hasFirst != w.firstToken {
		t.Errorf("first token seen = %v, want %v", hasFirst, w.firstToken)
	}
	if got.FirstTokenAt.After(got.LastTokenAt) {
		t.Errorf("FirstTokenAt %v after LastTokenAt %v", got.FirstTokenAt, got.LastTokenAt)
	}
	if got.EvalDuration != w.evalDuration {
		t.Errorf("EvalDuration = %v, want %v", got.EvalDuration, w.evalDuration)
	}
	if got.FinishReason != w.finishReason {
		t.Errorf("FinishReason = %q, want %q", got.FinishReason, w.finishReason)
	}
	if got.Complete != w.complete {
		t.Errorf("Complete = %v, want %v", got.Complete, w.complete)
	}
	if got.ErrorType != w.errorType {
		t.Errorf("ErrorType = %q, want %q", got.ErrorType, w.errorType)
	}
}

func TestDecodersInBandErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		kind   Kind
		format Format
		body   string
		want   string
	}{
		{
			name: "native error line after tokens", kind: KindNative, format: FormatNDJSON,
			body: `{"model":"m","response":"Hi","done":false}` + "\n" + `{"error":"runner terminated"}` + "\n",
			want: ErrorTypeStream,
		},
		{
			name: "anthropic error event", kind: KindAnthropic, format: FormatSSE,
			body: "event: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\",\"message\":\"x\"}}\n\n",
			want: "overloaded_error",
		},
		{
			name: "openai error chunk with unusable type", kind: KindOpenAI, format: FormatSSE,
			body: "data: {\"error\":{\"message\":\"boom\",\"type\":\"Some Type!\"}}\n\n",
			want: ErrorTypeStream,
		},
		{
			name: "responses failed", kind: KindResponses, format: FormatSSE,
			body: "data: {\"type\":\"response.failed\",\"response\":{\"model\":\"m\",\"status\":\"failed\"}}\n\n",
			want: ErrorTypeStream,
		},
		{
			name: "responses error event", kind: KindResponses, format: FormatSSE,
			body: "data: {\"type\":\"error\",\"error\":{\"type\":\"server_error\"}}\n\n",
			want: "server_error",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := replay(t, tt.kind, tt.format, []byte(tt.body), 5)
			if got.ErrorType != tt.want {
				t.Errorf("ErrorType = %q, want %q", got.ErrorType, tt.want)
			}
		})
	}
}

func TestNativeStreamWithoutDoneIsIncomplete(t *testing.T) {
	t.Parallel()
	body := `{"model":"m","response":"Hi","done":false}` + "\n" + `{"model":"m","response":" there","done":false}`
	got := replay(t, KindNative, FormatNDJSON, []byte(body), 3)
	if got.Complete || got.TokensReported {
		t.Errorf("Complete=%v TokensReported=%v, want both false", got.Complete, got.TokensReported)
	}
	if got.FirstTokenAt.IsZero() || !got.LastTokenAt.After(got.FirstTokenAt) {
		t.Errorf("token times first=%v last=%v, want two distinct times", got.FirstTokenAt, got.LastTokenAt)
	}
}

func TestNewDecoderNone(t *testing.T) {
	t.Parallel()
	if d := NewDecoder(KindNone); d != nil {
		t.Errorf("NewDecoder(KindNone) = %T, want nil", d)
	}
}
