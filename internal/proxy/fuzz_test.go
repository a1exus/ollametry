package proxy

import (
	"encoding/json"
	"testing"
)

// FuzzModelFromPrefix checks that the truncated-body walk agrees with
// encoding/json, which Ollama uses, on every valid body, and that sanitized
// names stay bounded.
func FuzzModelFromPrefix(f *testing.F) {
	f.Add([]byte(`{"model":"granite4.2:8b","prompt":"hi"}`))
	f.Add([]byte(`{"messages":[{"role":"user","content":"{\"model\":\"x\"}"}],"model":"m"}`))
	f.Add([]byte(`{"options":{"model":"nested"},"model":"top"}`))
	f.Add([]byte(`{"prompt":"a very long prompt that gets cut`))
	f.Add([]byte(`{"model":7}`))
	f.Add([]byte(`[{"model":"m"}]`))
	f.Add([]byte(`{"Model":"upper","prompt":"hi"}`))
	f.Add([]byte(`{"model":"first","model":"last"}`))
	f.Add([]byte(`{"model":"a","model":null}`))

	f.Fuzz(func(t *testing.T, body []byte) {
		prefix := modelFromPrefix(body)
		if len(sanitizeModel(prefix)) > maxModelNameLen {
			t.Fatalf("sanitized model longer than %d bytes", maxModelNameLen)
		}
		if !json.Valid(body) {
			return
		}
		if full := modelFromBody(body); prefix != full {
			t.Fatalf("prefix walk found %q, encoding/json %q", prefix, full)
		}
	})
}
