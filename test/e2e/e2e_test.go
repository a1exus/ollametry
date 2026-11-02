//go:build e2e

// Package e2e tests the deployed system from a client's point of view: real
// requests through the ollametry container to a real Ollama, then the
// resulting series in otel-lgtm's Prometheus. Run it with `make e2e` (against
// the local stack) or `make e2e-hermetic` (a throwaway stack, as CI does).
package e2e

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

// config comes from the environment; the Makefile targets set it.
type config struct {
	proxyURL, adminURL, prometheusURL, model string
	// metricsTimeout bounds the wait for a series: export interval plus
	// Prometheus ingestion.
	metricsTimeout time.Duration
}

func loadConfig(t *testing.T) config {
	t.Helper()
	get := func(key, fallback string) string {
		if v := os.Getenv(key); v != "" {
			return v
		}
		return fallback
	}
	timeout, err := time.ParseDuration(get("E2E_METRICS_TIMEOUT", "90s"))
	if err != nil {
		t.Fatalf("E2E_METRICS_TIMEOUT: %v", err)
	}
	cfg := config{
		proxyURL:       get("E2E_PROXY_URL", "http://ollametry:11435"),
		adminURL:       get("E2E_ADMIN_URL", "http://ollametry:11436"),
		prometheusURL:  get("E2E_PROMETHEUS_URL", "http://otel-lgtm:9090"),
		model:          os.Getenv("E2E_MODEL"),
		metricsTimeout: timeout,
	}
	if cfg.model == "" {
		t.Fatal("E2E_MODEL is required: an installed model to send requests to")
	}
	return cfg
}

var client = &http.Client{Timeout: 5 * time.Minute}

func do(t *testing.T, method, rawURL, body string, headers map[string]string) (int, []byte) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, rawURL, strings.NewReader(body))
	if err != nil {
		t.Fatalf("build %s %s: %v", method, rawURL, err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, rawURL, err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read %s %s: %v", method, rawURL, err)
	}
	return resp.StatusCode, b
}

func waitReady(t *testing.T, cfg config) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Minute)
	for {
		req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, cfg.adminURL+"/readyz", nil)
		if resp, err := client.Do(req); err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s/readyz not ready within 2m", cfg.adminURL)
		}
		time.Sleep(time.Second)
	}
}

// promValues runs an instant query and returns the value of each series,
// keyed by its full label set. Series are compared one by one, never summed:
// a replaced container's series linger for 5 minutes next to the new ones.
func promValues(t *testing.T, cfg config, query string) map[string]float64 {
	t.Helper()
	status, body := do(t, http.MethodGet, cfg.prometheusURL+"/api/v1/query?query="+url.QueryEscape(query), "", nil)
	if status != http.StatusOK {
		t.Fatalf("Prometheus query %q: status %d: %s", query, status, body)
	}
	var res struct {
		Data struct {
			Result []struct {
				Metric map[string]string `json:"metric"`
				Value  [2]any            `json:"value"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		t.Fatalf("decode Prometheus response for %q: %v", query, err)
	}
	out := map[string]float64{}
	for _, r := range res.Data.Result {
		s, _ := r.Value[1].(string)
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			t.Fatalf("value %q of %q: %v", s, query, err)
		}
		out[labelKey(r.Metric)] = v
	}
	return out
}

func labelKey(labels map[string]string) string {
	pairs := make([]string, 0, len(labels))
	for k, v := range labels {
		pairs = append(pairs, k+"="+v)
	}
	sort.Strings(pairs)
	return strings.Join(pairs, ",")
}

// expectation is a query whose result must come to satisfy met. Exports and
// polls are periodic, so every check waits instead of reading once.
type expectation struct {
	name, query, want string
	met               func(values map[string]float64) bool
}

// grows expects some series of query to rise by at least min from now on.
func grows(t *testing.T, cfg config, name, query string, min float64) *expectation {
	t.Helper()
	before := promValues(t, cfg, query)
	return &expectation{
		name: name, query: query, want: fmt.Sprintf("a series to grow by %v", min),
		met: func(values map[string]float64) bool {
			for k, v := range values {
				if v-before[k] >= min {
					return true
				}
			}
			return false
		},
	}
}

// equals expects some series of query to have the value want.
func equals(name, query string, want float64) *expectation {
	return &expectation{
		name: name, query: query, want: fmt.Sprintf("a series equal to %v", want),
		met: func(values map[string]float64) bool {
			for _, v := range values {
				if v == want {
					return true
				}
			}
			return false
		},
	}
}

// present expects query to return at least one series.
func present(name, query string) *expectation {
	return &expectation{
		name: name, query: query, want: "at least one series",
		met: func(values map[string]float64) bool { return len(values) > 0 },
	}
}

func waitAll(t *testing.T, cfg config, exps []*expectation) {
	t.Helper()
	deadline := time.Now().Add(cfg.metricsTimeout)
	pending := exps
	for len(pending) > 0 {
		var still []*expectation
		for _, e := range pending {
			if !e.met(promValues(t, cfg, e.query)) {
				still = append(still, e)
			}
		}
		pending = still
		if len(pending) == 0 {
			return
		}
		if time.Now().After(deadline) {
			for _, e := range pending {
				t.Errorf("%s: want %s within %v\n  query: %s", e.name, e.want, cfg.metricsTimeout, e.query)
			}
			return
		}
		time.Sleep(2 * time.Second)
	}
}

// apiCase is one request through the proxy and the series it must produce.
type apiCase struct {
	name, path, operation, body string
	headers                     map[string]string
	// check validates the response the client received.
	check func(t *testing.T, body []byte)
	// streamed requests must also record time to first token.
	streamed bool
}

func apiCases(model string) []apiCase {
	const prompt = "Reply with one word."
	return []apiCase{
		{
			name: "native chat stream", path: "/api/chat", operation: "chat", streamed: true,
			body:  fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":%q}],"options":{"num_predict":8}}`, model, prompt),
			check: lastNDJSON(func(m map[string]any) bool { return m["done"] == true && m["eval_count"] != nil }),
		},
		{
			name: "native generate", path: "/api/generate", operation: "text_completion",
			body:  fmt.Sprintf(`{"model":%q,"prompt":%q,"stream":false,"options":{"num_predict":8}}`, model, prompt),
			check: jsonHas("done", "eval_count"),
		},
		{
			name: "openai chat stream", path: "/v1/chat/completions", operation: "chat", streamed: true,
			body:  fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":%q}],"max_tokens":8,"stream":true,"stream_options":{"include_usage":true}}`, model, prompt),
			check: sseHas(`"usage":{`, "data: [DONE]"),
		},
		{
			name: "anthropic messages stream", path: "/v1/messages", operation: "chat", streamed: true,
			headers: map[string]string{"anthropic-version": "2023-06-01"},
			body:    fmt.Sprintf(`{"model":%q,"max_tokens":8,"stream":true,"messages":[{"role":"user","content":%q}]}`, model, prompt),
			check:   sseHas("event: message_delta", "event: message_stop"),
		},
		{
			name: "responses stream", path: "/v1/responses", operation: "chat", streamed: true,
			body:  fmt.Sprintf(`{"model":%q,"input":%q,"max_output_tokens":16,"stream":true}`, model, prompt),
			check: sseHas("response.completed"),
		},
	}
}

func lastNDJSON(ok func(map[string]any) bool) func(*testing.T, []byte) {
	return func(t *testing.T, body []byte) {
		t.Helper()
		var last map[string]any
		sc := bufio.NewScanner(bytes.NewReader(body))
		sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
		lines := 0
		for sc.Scan() {
			if len(bytes.TrimSpace(sc.Bytes())) == 0 {
				continue
			}
			lines++
			last = nil
			if err := json.Unmarshal(sc.Bytes(), &last); err != nil {
				t.Fatalf("line %d is not JSON: %v", lines, err)
			}
		}
		if lines < 2 || !ok(last) {
			t.Fatalf("want a stream of NDJSON lines ending in a final record, got %d lines, last %v", lines, last)
		}
	}
}

func jsonHas(keys ...string) func(*testing.T, []byte) {
	return func(t *testing.T, body []byte) {
		t.Helper()
		var m map[string]any
		if err := json.Unmarshal(body, &m); err != nil {
			t.Fatalf("response is not JSON: %v", err)
		}
		for _, k := range keys {
			if _, ok := m[k]; !ok {
				t.Errorf("response lacks %q: %s", k, body)
			}
		}
	}
}

func sseHas(markers ...string) func(*testing.T, []byte) {
	return func(t *testing.T, body []byte) {
		t.Helper()
		for _, m := range markers {
			if !bytes.Contains(body, []byte(m)) {
				t.Errorf("event stream lacks %q", m)
			}
		}
	}
}

func selector(labels map[string]string) string {
	parts := make([]string, 0, len(labels))
	for k, v := range labels {
		parts = append(parts, fmt.Sprintf("%s=%q", k, v))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func TestE2E(t *testing.T) {
	cfg := loadConfig(t)
	waitReady(t, cfg)
	cases := apiCases(cfg.model)

	// Baselines first, so a single wait covers every request.
	var exps []*expectation
	add := func(name, query string, min float64) {
		exps = append(exps, grows(t, cfg, name, query, min))
	}
	for _, c := range cases {
		labels := selector(map[string]string{
			"gen_ai_request_model": cfg.model, "http_route": c.path, "gen_ai_operation_name": c.operation,
		})
		add(c.name+": operation duration", "gen_ai_client_operation_duration_seconds_count"+labels, 1)
		for _, typ := range []string{"input", "output"} {
			tl := selector(map[string]string{"gen_ai_request_model": cfg.model, "http_route": c.path, "gen_ai_token_type": typ})
			add(c.name+": "+typ+" tokens", "gen_ai_client_token_usage_sum"+tl, 1)
		}
		if c.streamed {
			add(c.name+": time to first token", "gen_ai_server_time_to_first_token_seconds_count"+labels, 1)
		}
	}
	const unknown = "e2e-no-such-model:latest"
	add("unknown model: folded into _OTHER with error.type 404",
		`gen_ai_client_operation_duration_seconds_count{gen_ai_request_model="_OTHER",error_type="404",http_route="/api/chat"}`, 1)

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			status, body := do(t, http.MethodPost, cfg.proxyURL+c.path, c.body, c.headers)
			if status != http.StatusOK {
				t.Fatalf("status %d, want 200: %s", status, body)
			}
			c.check(t, body)
		})
	}
	t.Run("unknown model", func(t *testing.T) {
		status, body := do(t, http.MethodPost, cfg.proxyURL+"/api/chat",
			fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"hi"}]}`, unknown), nil)
		if status != http.StatusNotFound || !bytes.Contains(body, []byte(`"error"`)) {
			t.Fatalf("status %d body %s, want Ollama's 404 error passed through", status, body)
		}
	})

	t.Run("request metrics reach Prometheus", func(t *testing.T) {
		waitAll(t, cfg, exps)
	})
	t.Run("gauges reach Prometheus", func(t *testing.T) {
		// Fresh samples only: a gauge keeps its last value for 5 minutes
		// after it stops being exported.
		fresh := func(metric, labels string) string {
			return fmt.Sprintf("%[1]s%[2]s and (time() - timestamp(%[1]s%[2]s) < 60)", metric, labels)
		}
		loaded := selector(map[string]string{"ollama_model_name": cfg.model})
		waitAll(t, cfg, []*expectation{
			equals("ollama.up", fresh("ollama_up", ""), 1),
			present("ollama.build.info", fresh("ollama_build_info", "")),
			present("memory of the model the requests loaded", fresh("ollama_model_memory_size_bytes", loaded)),
		})
	})
}
