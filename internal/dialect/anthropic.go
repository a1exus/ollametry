package dialect

import (
	"encoding/json"
	"time"
)

// anthropicDoc covers Messages API responses and stream events.
type anthropicDoc struct {
	Type       string          `json:"type"`
	Model      string          `json:"model"`
	StopReason string          `json:"stop_reason"`
	Usage      *anthropicUsage `json:"usage"`
	Message    *struct {
		Model string          `json:"model"`
		Usage *anthropicUsage `json:"usage"`
	} `json:"message"`
	Delta *struct {
		StopReason string `json:"stop_reason"`
	} `json:"delta"`
	Error *typedError `json:"error"`
}

// anthropicUsage fields are pointers because a message_delta event updates
// only the counts it carries.
type anthropicUsage struct {
	InputTokens              *int64 `json:"input_tokens"`
	CacheReadInputTokens     *int64 `json:"cache_read_input_tokens"`
	CacheCreationInputTokens *int64 `json:"cache_creation_input_tokens"`
	OutputTokens             *int64 `json:"output_tokens"`
}

// anthropicDecoder keeps the usage components because the API reports
// cached input separately from input_tokens and may update them piecemeal.
type anthropicDecoder struct {
	input, cacheRead, cacheCreation, output int64
}

func (a *anthropicDecoder) Decode(doc []byte, at time.Time, r *Result) {
	var d anthropicDoc
	if err := json.Unmarshal(doc, &d); err != nil {
		return
	}
	r.Documents++

	switch d.Type {
	case "message": // non-streaming response
		r.setModel(d.Model)
		r.FinishReason = d.StopReason
		a.apply(d.Usage, r)
		r.Complete = true
	case "message_start":
		if d.Message != nil {
			r.setModel(d.Message.Model)
			a.apply(d.Message.Usage, r)
		}
	case "content_block_delta":
		r.markToken(at)
	case "message_delta":
		if d.Delta != nil && d.Delta.StopReason != "" {
			r.FinishReason = d.Delta.StopReason
		}
		a.apply(d.Usage, r)
	case "message_stop":
		r.Complete = true
	case "error":
		r.ErrorType = ErrorTypeStream
		if d.Error != nil {
			r.ErrorType = errorType(d.Error.Type)
		}
	}
}

func (a *anthropicDecoder) apply(u *anthropicUsage, r *Result) {
	if u == nil {
		return
	}
	set := func(dst *int64, src *int64) {
		if src != nil {
			*dst = *src
		}
	}
	set(&a.input, u.InputTokens)
	set(&a.cacheRead, u.CacheReadInputTokens)
	set(&a.cacheCreation, u.CacheCreationInputTokens)
	set(&a.output, u.OutputTokens)

	r.TokensReported = true
	r.InputTokens = a.input + a.cacheRead + a.cacheCreation
	r.CachedInputTokens = a.cacheRead
	r.OutputTokens = a.output
}
