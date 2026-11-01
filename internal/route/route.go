// Package route classifies Ollama API requests: the low-cardinality
// http.route template for metrics and, for inference endpoints, the GenAI
// operation and the response dialect.
package route

import (
	"net/http"
	"strings"

	"github.com/a1exus/ollametry/internal/dialect"
	"github.com/a1exus/ollametry/internal/genai"
)

// Route describes a matched request.
type Route struct {
	// Pattern is the http.route value; empty when the path is not a known
	// Ollama route, so that arbitrary paths cannot inflate metric cardinality.
	Pattern string
	// Operation is the gen_ai.operation.name; empty for non-inference routes.
	Operation string
	// Dialect selects the response decoder.
	Dialect dialect.Kind
}

// Inference reports whether the request runs a model.
func (r Route) Inference() bool {
	return r.Operation != ""
}

var inference = map[string]Route{
	"/api/generate":        {Pattern: "/api/generate", Operation: genai.OperationTextCompletion, Dialect: dialect.KindNative},
	"/api/chat":            {Pattern: "/api/chat", Operation: genai.OperationChat, Dialect: dialect.KindNative},
	"/api/embed":           {Pattern: "/api/embed", Operation: genai.OperationEmbeddings, Dialect: dialect.KindNative},
	"/api/embeddings":      {Pattern: "/api/embeddings", Operation: genai.OperationEmbeddings, Dialect: dialect.KindNative},
	"/v1/chat/completions": {Pattern: "/v1/chat/completions", Operation: genai.OperationChat, Dialect: dialect.KindOpenAI},
	"/v1/completions":      {Pattern: "/v1/completions", Operation: genai.OperationTextCompletion, Dialect: dialect.KindOpenAI},
	"/v1/embeddings":       {Pattern: "/v1/embeddings", Operation: genai.OperationEmbeddings, Dialect: dialect.KindOpenAI},
	"/v1/messages":         {Pattern: "/v1/messages", Operation: genai.OperationChat, Dialect: dialect.KindAnthropic},
	"/v1/responses":        {Pattern: "/v1/responses", Operation: genai.OperationChat, Dialect: dialect.KindResponses},
}

var static = map[string]bool{
	"/":                         true,
	"/api/version":              true,
	"/api/tags":                 true,
	"/api/ps":                   true,
	"/api/show":                 true,
	"/api/pull":                 true,
	"/api/push":                 true,
	"/api/create":               true,
	"/api/copy":                 true,
	"/api/delete":               true,
	"/api/me":                   true,
	"/api/signin":               true,
	"/api/signout":              true,
	"/v1/models":                true,
	"/v1/messages/count_tokens": true,
}

// templates maps a path prefix to the route template of its single
// trailing parameter.
var templates = []struct{ prefix, pattern string }{
	{"/api/blobs/", "/api/blobs/{digest}"},
	{"/v1/models/", "/v1/models/{model}"},
}

// Match classifies a request by method and path.
func Match(method, path string) Route {
	if r, ok := inference[path]; ok {
		if method == http.MethodPost {
			return r
		}
		return Route{Pattern: r.Pattern}
	}
	if static[path] {
		return Route{Pattern: path}
	}
	for _, t := range templates {
		if rest, ok := strings.CutPrefix(path, t.prefix); ok && rest != "" && !strings.Contains(rest, "/") {
			return Route{Pattern: t.pattern}
		}
	}
	return Route{}
}
