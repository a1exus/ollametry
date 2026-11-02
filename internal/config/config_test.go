package config

import (
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		args    []string
		env     map[string]string
		wantErr []string
		check   func(t *testing.T, c Config)
	}{
		{
			name: "defaults",
			check: func(t *testing.T, c Config) {
				t.Helper()
				if c.ListenAddr != DefaultListenAddr || c.AdminAddr != DefaultAdminAddr ||
					c.OllamaURL.String() != DefaultOllamaURL || c.PollInterval != DefaultPollInterval ||
					c.MaxInspectBytes != DefaultMaxInspectBytes || c.LogLevel != slog.LevelInfo || c.LogFormat != "json" ||
					!c.Prometheus {
					t.Errorf("defaults = %+v", c)
				}
			},
		},
		{
			name: "environment",
			env: map[string]string{
				"OLLAMETRY_OLLAMA_URL":    "http://host.docker.internal:11434",
				"OLLAMETRY_POLL_INTERVAL": "5s",
				"OLLAMETRY_LOG_LEVEL":     "DEBUG",
				"OLLAMETRY_LOG_FORMAT":    "TEXT",
			},
			check: func(t *testing.T, c Config) {
				t.Helper()
				if c.OllamaURL.Host != "host.docker.internal:11434" || c.PollInterval != 5*time.Second ||
					c.LogLevel != slog.LevelDebug || c.LogFormat != "text" {
					t.Errorf("config = %+v", c)
				}
			},
		},
		{
			name: "flags win over environment",
			args: []string{"-listen", ":9000", "-poll-interval", "1m"},
			env:  map[string]string{"OLLAMETRY_LISTEN_ADDR": ":8000", "OLLAMETRY_POLL_INTERVAL": "5s"},
			check: func(t *testing.T, c Config) {
				t.Helper()
				if c.ListenAddr != ":9000" || c.PollInterval != time.Minute {
					t.Errorf("config = %+v", c)
				}
			},
		},
		{name: "bad url scheme", args: []string{"-ollama-url", "ftp://x"}, wantErr: []string{"ollama-url"}},
		{name: "relative url", args: []string{"-ollama-url", "localhost:11434"}, wantErr: []string{"ollama-url"}},
		{name: "bad listen", args: []string{"-listen", "11435"}, wantErr: []string{"listen"}},
		{name: "bad port", args: []string{"-listen", ":99999"}, wantErr: []string{"invalid port"}},
		{name: "zero interval", args: []string{"-poll-interval", "0s"}, wantErr: []string{"poll-interval"}},
		{name: "negative inspect", args: []string{"-max-inspect-bytes", "-1"}, wantErr: []string{"max-inspect-bytes"}},
		{name: "bad log level", args: []string{"-log-level", "loud"}, wantErr: []string{"log-level"}},
		{name: "bad log format", args: []string{"-log-format", "xml"}, wantErr: []string{"log-format"}},
		{name: "bad prometheus switch", args: []string{"-prometheus", "maybe"}, wantErr: []string{"prometheus"}},
		{
			name: "prometheus off",
			env:  map[string]string{"OLLAMETRY_PROMETHEUS": "false"},
			check: func(t *testing.T, c Config) {
				t.Helper()
				if c.Prometheus {
					t.Error("Prometheus = true with OLLAMETRY_PROMETHEUS=false")
				}
			},
		},
		{name: "same addresses", args: []string{"-listen", ":1", "-admin-listen", ":1"}, wantErr: []string{"also the proxy"}},
		{name: "positional argument", args: []string{"serve"}, wantErr: []string{"unexpected arguments"}},
		{name: "unknown flag", args: []string{"-nope"}, wantErr: []string{"parse flags"}},
		{
			name:    "all errors reported together",
			args:    []string{"-poll-interval", "x", "-log-format", "xml"},
			wantErr: []string{"poll-interval", "log-format"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			getenv := func(k string) string { return tt.env[k] }
			c, err := Load(tt.args, getenv, io.Discard)
			if len(tt.wantErr) > 0 {
				if err == nil {
					t.Fatalf("Load() error = nil, want one mentioning %q", tt.wantErr)
				}
				for _, want := range tt.wantErr {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("Load() error = %v, want it to mention %q", err, want)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			tt.check(t, c)
		})
	}
}
