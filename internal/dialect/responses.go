package dialect

import (
	"encoding/json"
	"strings"
	"time"
)

// responsesDoc covers Responses API objects and stream events. A stream event
// wraps the response object in "response"; a non-streaming body is the
// response object itself.
type responsesDoc struct {
	Type     string             `json:"type"`
	Object   string             `json:"object"`
	Response *responsesResponse `json:"response"`
	Error    *typedError        `json:"error"`
	responsesResponse
}

type responsesResponse struct {
	Model             string          `json:"model"`
	Status            string          `json:"status"`
	Usage             *responsesUsage `json:"usage"`
	IncompleteDetails *struct {
		Reason string `json:"reason"`
	} `json:"incomplete_details"`
}

type responsesUsage struct {
	InputTokens        int64 `json:"input_tokens"`
	OutputTokens       int64 `json:"output_tokens"`
	InputTokensDetails *struct {
		CachedTokens int64 `json:"cached_tokens"`
	} `json:"input_tokens_details"`
}

type responsesDecoder struct{}

func (responsesDecoder) Decode(doc []byte, at time.Time, r *Result) {
	var d responsesDoc
	if err := json.Unmarshal(doc, &d); err != nil {
		return
	}
	r.Documents++

	switch {
	case d.Object == "response": // non-streaming response
		finish(&d.responsesResponse, r)
	case strings.HasSuffix(d.Type, ".delta"):
		// output_text, refusal, reasoning, reasoning_summary_text and
		// function_call_arguments deltas all mean the model produced output.
		r.markToken(at)
	case d.Type == "response.completed", d.Type == "response.incomplete", d.Type == "response.failed":
		if d.Response != nil {
			finish(d.Response, r)
		}
	case d.Type == "response.created", d.Type == "response.in_progress":
		if d.Response != nil {
			r.setModel(d.Response.Model)
		}
	case d.Type == "error":
		r.ErrorType = ErrorTypeStream
		if d.Error != nil {
			r.ErrorType = errorType(d.Error.Type)
		}
	}
}

func finish(resp *responsesResponse, r *Result) {
	r.setModel(resp.Model)
	if u := resp.Usage; u != nil {
		r.TokensReported = true
		r.InputTokens = u.InputTokens
		r.OutputTokens = u.OutputTokens
		if u.InputTokensDetails != nil {
			r.CachedInputTokens = u.InputTokensDetails.CachedTokens
		}
	}
	switch resp.Status {
	case "failed":
		r.ErrorType = ErrorTypeStream
	case "incomplete":
		if resp.IncompleteDetails != nil {
			r.FinishReason = resp.IncompleteDetails.Reason
		}
	}
	r.FinishReason = firstNonEmpty(r.FinishReason, resp.Status)
	r.Complete = true
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
