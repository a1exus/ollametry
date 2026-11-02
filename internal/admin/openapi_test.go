package admin

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// spec is the part of api/openapi.yaml the contract test checks.
type spec struct {
	Paths map[string]map[string]struct {
		Responses map[string]specResponse `yaml:"responses"`
	} `yaml:"paths"`
	Components struct {
		Responses map[string]specResponse `yaml:"responses"`
		Headers   map[string]specHeader   `yaml:"headers"`
	} `yaml:"components"`
}

type specResponse struct {
	Ref     string                `yaml:"$ref"`
	Headers map[string]specHeader `yaml:"headers"`
	Content map[string]struct {
		Schema struct {
			Const *string `yaml:"const"`
		} `yaml:"schema"`
	} `yaml:"content"`
}

type specHeader struct {
	Ref    string `yaml:"$ref"`
	Schema struct {
		Const *string `yaml:"const"`
	} `yaml:"schema"`
}

func loadSpec(t *testing.T) spec {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "api", "openapi.yaml"))
	if err != nil {
		t.Fatalf("read api/openapi.yaml: %v", err)
	}
	var s spec
	if err := yaml.Unmarshal(raw, &s); err != nil {
		t.Fatalf("parse api/openapi.yaml: %v", err)
	}
	return s
}

func (s spec) response(r specResponse) specResponse {
	if r.Ref == "" {
		return r
	}
	return s.Components.Responses[strings.TrimPrefix(r.Ref, "#/components/responses/")]
}

func (s spec) header(h specHeader) specHeader {
	if h.Ref == "" {
		return h
	}
	return s.Components.Headers[strings.TrimPrefix(h.Ref, "#/components/headers/")]
}

// scenario is the handler state and request that should produce a
// documented status.
func scenario(t *testing.T, path string, status int) (method string, ready bool, metrics http.Handler) {
	t.Helper()
	fakeMetrics := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_, _ = w.Write([]byte("ollama_up 1\n"))
	})
	switch {
	case status == http.StatusMethodNotAllowed:
		return http.MethodPost, true, fakeMetrics
	case path == "/readyz" && status == http.StatusServiceUnavailable:
		return http.MethodGet, false, fakeMetrics
	case path == "/metrics" && status == http.StatusNotFound:
		return http.MethodGet, true, nil
	case status == http.StatusOK:
		return http.MethodGet, true, fakeMetrics
	}
	t.Fatalf("no scenario for %s %d: add one when documenting a new status", path, status)
	return "", false, nil
}

// TestHandlerMatchesOpenAPI requests every documented path and status from
// the real handler, so api/openapi.yaml cannot drift from the code.
func TestHandlerMatchesOpenAPI(t *testing.T) {
	t.Parallel()
	s := loadSpec(t)
	if len(s.Paths) == 0 {
		t.Fatal("api/openapi.yaml documents no paths")
	}
	for path, ops := range s.Paths {
		op, ok := ops["get"]
		if !ok || len(ops) != 1 {
			t.Errorf("%s: want exactly a get operation (HEAD and 405 are implied), got %v", path, slices.Collect(mapKeys(ops)))
			continue
		}
		for code, documented := range op.Responses {
			t.Run(path+" "+code, func(t *testing.T) {
				t.Parallel()
				status, err := strconv.Atoi(code)
				if err != nil {
					t.Fatalf("status %q is not a number", code)
				}
				method, ready, metrics := scenario(t, path, status)
				rec := httptest.NewRecorder()
				Handler(func() bool { return ready }, metrics).ServeHTTP(rec, httptest.NewRequest(method, path, nil))

				if rec.Code != status {
					t.Fatalf("%s %s = %d, want documented %d", method, path, rec.Code, status)
				}
				resp := s.response(documented)
				for name, h := range resp.Headers {
					want := s.header(h).Schema.Const
					if got := rec.Header().Get(name); got == "" || (want != nil && got != *want) {
						t.Errorf("header %s = %q, want %v", name, got, derefOr(want, "present"))
					}
				}
				for mediaType, content := range resp.Content {
					if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, mediaType) {
						t.Errorf("Content-Type = %q, want %s", got, mediaType)
					}
					if want := content.Schema.Const; want != nil && rec.Body.String() != *want {
						t.Errorf("body = %q, want %q", rec.Body.String(), *want)
					}
				}
			})
		}
		t.Run(path+" HEAD", func(t *testing.T) {
			t.Parallel()
			rec := httptest.NewRecorder()
			Handler(func() bool { return true }, http.NotFoundHandler()).ServeHTTP(rec, httptest.NewRequest(http.MethodHead, path, nil))
			if rec.Code == http.StatusMethodNotAllowed {
				t.Errorf("HEAD %s = 405; the spec says every operation answers HEAD", path)
			}
		})
	}
}

func mapKeys[M ~map[string]V, V any](m M) func(func(string) bool) {
	return func(yield func(string) bool) {
		for k := range m {
			if !yield(k) {
				return
			}
		}
	}
}

func derefOr(s *string, fallback string) string {
	if s == nil {
		return fallback
	}
	return *s
}
