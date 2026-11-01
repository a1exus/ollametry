// Package ollama polls Ollama's management API and exposes what it learns as
// OpenTelemetry gauges: whether the server is up, which models are loaded and
// how much memory they hold, and what is installed.
package ollama

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Client is a minimal client for the read-only endpoints the poller needs.
type Client struct {
	base *url.URL
	http *http.Client
}

// NewClient returns a Client for the Ollama server at base.
func NewClient(base *url.URL, httpClient *http.Client) *Client {
	return &Client{base: base, http: httpClient}
}

// RunningModel is one entry of GET /api/ps.
type RunningModel struct {
	Name          string       `json:"name"`
	Size          int64        `json:"size"`
	SizeVRAM      int64        `json:"size_vram"`
	ContextLength int64        `json:"context_length"`
	ExpiresAt     time.Time    `json:"expires_at"`
	Runner        string       `json:"runner"`
	Details       ModelDetails `json:"details"`
}

// InstalledModel is one entry of GET /api/tags.
type InstalledModel struct {
	Name       string       `json:"name"`
	Size       int64        `json:"size"`
	RemoteHost string       `json:"remote_host"`
	Details    ModelDetails `json:"details"`
}

// ModelDetails is the "details" object shared by /api/ps and /api/tags.
type ModelDetails struct {
	Family            string `json:"family"`
	ParameterSize     string `json:"parameter_size"`
	QuantizationLevel string `json:"quantization_level"`
	Runner            string `json:"runner"`
}

// Version returns the server version from GET /api/version.
func (c *Client) Version(ctx context.Context) (string, error) {
	var v struct {
		Version string `json:"version"`
	}
	if err := c.get(ctx, "/api/version", &v); err != nil {
		return "", err
	}
	return v.Version, nil
}

// Running returns the loaded models from GET /api/ps.
func (c *Client) Running(ctx context.Context) ([]RunningModel, error) {
	var v struct {
		Models []RunningModel `json:"models"`
	}
	if err := c.get(ctx, "/api/ps", &v); err != nil {
		return nil, err
	}
	return v.Models, nil
}

// Installed returns the installed models from GET /api/tags.
func (c *Client) Installed(ctx context.Context) ([]InstalledModel, error) {
	var v struct {
		Models []InstalledModel `json:"models"`
	}
	if err := c.get(ctx, "/api/tags", &v); err != nil {
		return nil, err
	}
	return v.Models, nil
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	u := c.base.JoinPath(path)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return fmt.Errorf("build GET %s: %w", path, err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", u, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("GET %s: status %d, want 200: %s", u, resp.StatusCode, snippet)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode GET %s response: %w", u, err)
	}
	return nil
}
