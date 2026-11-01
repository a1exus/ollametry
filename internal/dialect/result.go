// Package dialect extracts usage and timing from the response bodies of
// Ollama's native, OpenAI-compatible and Anthropic-compatible APIs as they
// stream through the proxy.
package dialect

import (
	"encoding/json"
	"regexp"
	"time"
)

// Kind selects the decoder for an API family.
type Kind int

const (
	// KindNone marks routes whose bodies carry no inference results.
	KindNone Kind = iota
	// KindNative is /api/generate, /api/chat, /api/embed and /api/embeddings.
	KindNative
	// KindOpenAI is /v1/chat/completions, /v1/completions and /v1/embeddings.
	KindOpenAI
	// KindAnthropic is /v1/messages.
	KindAnthropic
	// KindResponses is /v1/responses.
	KindResponses
)

// ErrorTypeStream is the error.type for an error reported inside a response
// that started successfully, when the API gives no usable error type.
const ErrorTypeStream = "stream_error"

// Result accumulates what a response reveals about one inference request.
type Result struct {
	// ResponseModel is the model name the server reports.
	ResponseModel string

	// TokensReported is true once the response carried token counts; the
	// counts below are meaningless until then.
	TokensReported bool
	// InputTokens includes cached input tokens, whatever the API's convention.
	InputTokens       int64
	CachedInputTokens int64
	OutputTokens      int64

	// FirstTokenAt and LastTokenAt bracket the arrival of generated content,
	// reasoning included. Only meaningful for streaming formats.
	FirstTokenAt time.Time
	LastTokenAt  time.Time

	// Server-side timings, when the API reports them.
	LoadDuration       time.Duration
	PromptEvalDuration time.Duration
	EvalDuration       time.Duration

	// FinishReason is the API's stop reason, for logs.
	FinishReason string
	// Complete is set once the record that ends a successful response arrived.
	Complete bool
	// ErrorType is set when the body reported an error.
	ErrorType string
	// Documents counts the JSON documents decoded successfully.
	Documents int
}

func (r *Result) markToken(at time.Time) {
	if r.FirstTokenAt.IsZero() {
		r.FirstTokenAt = at
	}
	r.LastTokenAt = at
}

func (r *Result) setModel(model string) {
	if model != "" {
		r.ResponseModel = model
	}
}

// Decoder updates a Result from one framed document. Decoders are stateful
// and serve a single response.
type Decoder interface {
	Decode(doc []byte, at time.Time, r *Result)
}

// NewDecoder returns a fresh decoder for kind, or nil for KindNone.
func NewDecoder(kind Kind) Decoder {
	switch kind {
	case KindNative:
		return &nativeDecoder{}
	case KindOpenAI:
		return &openAIDecoder{}
	case KindAnthropic:
		return &anthropicDecoder{}
	case KindResponses:
		return &responsesDecoder{}
	case KindNone:
	}
	return nil
}

var errorTypePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// errorType turns an API-provided error type into a bounded error.type value.
func errorType(apiType string) string {
	if errorTypePattern.MatchString(apiType) {
		return apiType
	}
	return ErrorTypeStream
}

// typedError matches OpenAI and Anthropic error objects: {"type": "..."}.
type typedError struct {
	Type string `json:"type"`
}

// decodeErrorField returns the error.type for a raw "error" member: a string
// (Ollama native) or an object with a type (OpenAI, Anthropic). It returns ""
// for an absent or null member.
func decodeErrorField(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var obj typedError
	if err := json.Unmarshal(raw, &obj); err == nil {
		return errorType(obj.Type)
	}
	return ErrorTypeStream
}

func millis(ms float64) time.Duration {
	return time.Duration(ms * float64(time.Millisecond))
}
