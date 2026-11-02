package proxy

import (
	"context"
	"errors"
	"io"
	"strconv"
	"time"

	"github.com/a1exus/ollametry/internal/dialect"
	"github.com/a1exus/ollametry/internal/route"
)

// Values of error.type the proxy produces itself, besides HTTP status codes
// and the error types the APIs report.
const (
	ErrorTypeCanceled   = "canceled"
	ErrorTypeUpstream   = "upstream_error"
	ErrorTypeIncomplete = "incomplete"
)

// exchange is the state of one proxied request. It is only touched from the
// request's own goroutine: ReverseProxy calls Rewrite, ModifyResponse,
// ErrorHandler and reads the response body there.
type exchange struct {
	start  time.Time
	method string
	route  route.Route
	// requestModel is gen_ai.request.model; loggedModel is what the client
	// sent, which differs when the name is unknown to Ollama.
	requestModel string
	loggedModel  string

	status       int
	format       dialect.Format
	result       dialect.Result
	bodyEOF      bool
	bodyErr      error
	transportErr error
	// aborted is set when ReverseProxy gave up copying the body, which it
	// signals by panicking with http.ErrAbortHandler.
	aborted bool
}

type exchangeKey struct{}

func withExchange(ctx context.Context, ex *exchange) context.Context {
	return context.WithValue(ctx, exchangeKey{}, ex)
}

func exchangeFrom(ctx context.Context) *exchange {
	ex, _ := ctx.Value(exchangeKey{}).(*exchange)
	return ex
}

// errorType classifies how the exchange ended; "" means success. clientErr is
// the incoming request context's error once the exchange is over.
func (ex *exchange) errorType(clientErr error) string {
	switch {
	case clientErr != nil && !ex.bodyEOF, ex.aborted && ex.transportErr == nil && ex.bodyErr == nil:
		return ErrorTypeCanceled
	case ex.transportErr != nil, ex.bodyErr != nil:
		return ErrorTypeUpstream
	case ex.status >= 400:
		return strconv.Itoa(ex.status)
	case ex.result.ErrorType != "":
		return ex.result.ErrorType
	case ex.route.Inference() && ex.format.Streaming() && !ex.result.Complete:
		return ErrorTypeIncomplete
	}
	return ""
}

// timePerOutputToken prefers the server's own decode timing and falls back to
// the spacing of streamed tokens after the first one.
func (ex *exchange) timePerOutputToken() (time.Duration, bool) {
	r := &ex.result
	switch {
	case !r.TokensReported || r.OutputTokens <= 0:
		return 0, false
	case r.EvalDuration > 0:
		return r.EvalDuration / time.Duration(r.OutputTokens), true
	case ex.format.Streaming() && r.OutputTokens > 1 && r.LastTokenAt.After(r.FirstTokenAt):
		return r.LastTokenAt.Sub(r.FirstTokenAt) / time.Duration(r.OutputTokens-1), true
	}
	return 0, false
}

// tapBody relays an upstream response body while framing and decoding it.
type tapBody struct {
	io.ReadCloser
	ex     *exchange
	framer *dialect.Framer // nil when the body is not inspected
}

func (t *tapBody) Read(p []byte) (int, error) {
	n, err := t.ReadCloser.Read(p)
	if n > 0 && t.framer != nil {
		t.framer.Write(p[:n])
	}
	switch {
	case errors.Is(err, io.EOF):
		if !t.ex.bodyEOF && t.framer != nil {
			t.framer.Close()
		}
		t.ex.bodyEOF = true
	case err != nil && t.ex.bodyErr == nil:
		t.ex.bodyErr = err
	}
	return n, err //nolint:wrapcheck // io.Reader contract: errors pass through unchanged.
}
