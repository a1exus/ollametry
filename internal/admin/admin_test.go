package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandler(t *testing.T) {
	t.Parallel()
	tests := []struct {
		method, path string
		ready        bool
		want         int
	}{
		{http.MethodGet, "/livez", false, http.StatusOK},
		{http.MethodGet, "/readyz", true, http.StatusOK},
		{http.MethodGet, "/readyz", false, http.StatusServiceUnavailable},
		{http.MethodPost, "/livez", true, http.StatusMethodNotAllowed},
		{http.MethodGet, "/api/tags", true, http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.method+tt.path, func(t *testing.T) {
			t.Parallel()
			rec := httptest.NewRecorder()
			Handler(func() bool { return tt.ready }).ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, nil))
			if rec.Code != tt.want {
				t.Errorf("%s %s (ready=%v) = %d, want %d", tt.method, tt.path, tt.ready, rec.Code, tt.want)
			}
		})
	}
}
