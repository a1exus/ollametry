package route

import (
	"net/http"
	"testing"

	"github.com/a1exus/ollametry/internal/dialect"
	"github.com/a1exus/ollametry/internal/genai"
)

func TestMatch(t *testing.T) {
	t.Parallel()
	tests := []struct {
		method, path string
		want         Route
	}{
		{http.MethodPost, "/api/chat", Route{"/api/chat", genai.OperationChat, dialect.KindNative}},
		{http.MethodPost, "/api/generate", Route{"/api/generate", genai.OperationTextCompletion, dialect.KindNative}},
		{http.MethodPost, "/api/embed", Route{"/api/embed", genai.OperationEmbeddings, dialect.KindNative}},
		{http.MethodPost, "/v1/chat/completions", Route{"/v1/chat/completions", genai.OperationChat, dialect.KindOpenAI}},
		{http.MethodPost, "/v1/messages", Route{"/v1/messages", genai.OperationChat, dialect.KindAnthropic}},
		{http.MethodPost, "/v1/responses", Route{"/v1/responses", genai.OperationChat, dialect.KindResponses}},
		{http.MethodOptions, "/api/chat", Route{Pattern: "/api/chat"}},
		{http.MethodGet, "/api/tags", Route{Pattern: "/api/tags"}},
		{http.MethodPost, "/v1/messages/count_tokens", Route{Pattern: "/v1/messages/count_tokens"}},
		{http.MethodHead, "/api/blobs/sha256:abc", Route{Pattern: "/api/blobs/{digest}"}},
		{http.MethodGet, "/v1/models/llama3:8b", Route{Pattern: "/v1/models/{model}"}},
		{http.MethodGet, "/v1/models/", Route{}},
		{http.MethodGet, "/api/blobs/a/b", Route{}},
		{http.MethodGet, "/api/chat/", Route{}},
		{http.MethodGet, "/random/path", Route{}},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			t.Parallel()
			got := Match(tt.method, tt.path)
			if got != tt.want {
				t.Errorf("Match(%q, %q) = %+v, want %+v", tt.method, tt.path, got, tt.want)
			}
			if got.Inference() != (tt.want.Operation != "") {
				t.Errorf("Inference() = %v", got.Inference())
			}
		})
	}
}
