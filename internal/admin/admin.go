// Package admin serves ollametry's own health and metrics endpoints on a
// separate listener, so they can never shadow an Ollama route.
package admin

import (
	"net/http"
)

// Handler serves:
//
//	GET /livez   200 while the process is serving
//	GET /readyz  200 when ready() is true (Ollama reachable, not shutting down), else 503
//	GET /metrics Prometheus exposition, when metrics is not nil
func Handler(ready func() bool, metrics http.Handler) http.Handler {
	mux := http.NewServeMux()
	if metrics != nil {
		mux.Handle("GET /metrics", metrics)
	}
	mux.HandleFunc("GET /livez", func(w http.ResponseWriter, _ *http.Request) {
		write(w, http.StatusOK, "ok")
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		if ready() {
			write(w, http.StatusOK, "ok")
			return
		}
		write(w, http.StatusServiceUnavailable, "not ready")
	})
	return mux
}

func write(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body + "\n"))
}
