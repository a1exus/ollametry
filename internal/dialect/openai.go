package dialect

import (
	"bytes"
	"encoding/json"
	"time"
)

// openAIDoc covers chat completion and completion objects and chunks, and
// embedding responses, as served by Ollama's OpenAI-compatible API.
type openAIDoc struct {
	Model   string          `json:"model"`
	Choices []openAIChoice  `json:"choices"`
	Usage   *openAIUsage    `json:"usage"`
	Timings *llamaTimings   `json:"timings"`
	Error   json.RawMessage `json:"error"`
}

type openAIChoice struct {
	Text         string         `json:"text"`
	Delta        *openAIMessage `json:"delta"`
	FinishReason *string        `json:"finish_reason"`
}

type openAIMessage struct {
	Content          string            `json:"content"`
	Reasoning        string            `json:"reasoning"`
	ReasoningContent string            `json:"reasoning_content"`
	ToolCalls        []json.RawMessage `json:"tool_calls"`
}

func (m *openAIMessage) hasContent() bool {
	return m != nil && (m.Content != "" || m.Reasoning != "" || m.ReasoningContent != "" || len(m.ToolCalls) > 0)
}

type openAIUsage struct {
	PromptTokens        int64 `json:"prompt_tokens"`
	CompletionTokens    int64 `json:"completion_tokens"`
	PromptTokensDetails *struct {
		CachedTokens int64 `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
}

// llamaTimings is the llama.cpp-style "timings" object Ollama adds to the
// final usage chunk of a stream.
type llamaTimings struct {
	PromptMS    float64 `json:"prompt_ms"`
	PredictedMS float64 `json:"predicted_ms"`
}

var sseDone = []byte("[DONE]")

type openAIDecoder struct{}

func (openAIDecoder) Decode(doc []byte, at time.Time, r *Result) {
	if bytes.Equal(doc, sseDone) {
		r.Complete = true
		return
	}
	var d openAIDoc
	if err := json.Unmarshal(doc, &d); err != nil {
		return
	}
	r.Documents++
	r.setModel(d.Model)

	if et := decodeErrorField(d.Error); et != "" {
		r.ErrorType = et
		return
	}
	for _, c := range d.Choices {
		// Streamed chat chunks carry deltas, streamed completions carry text.
		// Non-streaming bodies also have text or message, but time to first
		// token is only recorded for streaming formats, so that is harmless.
		if c.Delta.hasContent() || c.Text != "" {
			r.markToken(at)
		}
		if c.FinishReason != nil && *c.FinishReason != "" {
			r.FinishReason = *c.FinishReason
		}
	}
	// Streamed completions repeat a zeroed usage object on every chunk; only
	// the final one carries the counts.
	if u := d.Usage; u != nil && (u.PromptTokens > 0 || u.CompletionTokens > 0) {
		r.TokensReported = true
		r.InputTokens = u.PromptTokens
		r.OutputTokens = u.CompletionTokens
		if u.PromptTokensDetails != nil {
			r.CachedInputTokens = u.PromptTokensDetails.CachedTokens
		}
	}
	if t := d.Timings; t != nil {
		r.PromptEvalDuration = millis(t.PromptMS)
		r.EvalDuration = millis(t.PredictedMS)
	}
}
