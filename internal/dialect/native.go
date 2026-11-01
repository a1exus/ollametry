package dialect

import (
	"encoding/json"
	"time"
)

// nativeDoc covers /api/generate and /api/chat records (streamed or not) and
// /api/embed responses. Durations are nanoseconds.
type nativeDoc struct {
	Model    string          `json:"model"`
	Response string          `json:"response"`
	Thinking string          `json:"thinking"`
	Message  *nativeMessage  `json:"message"`
	Error    json.RawMessage `json:"error"`

	Done       bool   `json:"done"`
	DoneReason string `json:"done_reason"`

	LoadDuration          int64  `json:"load_duration"`
	PromptEvalCount       *int64 `json:"prompt_eval_count"`
	PromptEvalCachedCount int64  `json:"prompt_eval_cached_count"`
	PromptEvalDuration    int64  `json:"prompt_eval_duration"`
	EvalCount             *int64 `json:"eval_count"`
	EvalDuration          int64  `json:"eval_duration"`
}

type nativeMessage struct {
	Content   string            `json:"content"`
	Thinking  string            `json:"thinking"`
	ToolCalls []json.RawMessage `json:"tool_calls"`
}

func (m *nativeMessage) hasContent() bool {
	return m != nil && (m.Content != "" || m.Thinking != "" || len(m.ToolCalls) > 0)
}

type nativeDecoder struct{}

func (nativeDecoder) Decode(doc []byte, at time.Time, r *Result) {
	var d nativeDoc
	if err := json.Unmarshal(doc, &d); err != nil {
		return
	}
	r.Documents++
	r.setModel(d.Model)

	if et := decodeErrorField(d.Error); et != "" {
		r.ErrorType = et
		return
	}
	if d.Response != "" || d.Thinking != "" || d.Message.hasContent() {
		r.markToken(at)
	}

	// Embedding responses have no "done" flag but carry the same stats.
	if d.Done || d.PromptEvalCount != nil || d.EvalCount != nil {
		r.LoadDuration = time.Duration(d.LoadDuration)
		r.PromptEvalDuration = time.Duration(d.PromptEvalDuration)
		r.EvalDuration = time.Duration(d.EvalDuration)
		if d.PromptEvalCount != nil {
			r.TokensReported = true
			r.InputTokens = *d.PromptEvalCount
			r.CachedInputTokens = d.PromptEvalCachedCount
		}
		if d.EvalCount != nil {
			r.TokensReported = true
			r.OutputTokens = *d.EvalCount
		}
	}
	if d.Done {
		r.Complete = true
		r.FinishReason = d.DoneReason
	}
}
