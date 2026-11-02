// Package config loads ollametry's settings from flags, with OLLAMETRY_*
// environment variables as defaults. Exporter settings are not here: the
// OpenTelemetry SDK reads the standard OTEL_* variables itself.
package config

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Config holds every setting.
type Config struct {
	ListenAddr       string
	AdminAddr        string
	OllamaURL        *url.URL
	PollInterval     time.Duration
	ShutdownTimeout  time.Duration
	MaxInspectBytes  int
	CardinalityLimit int
	// Prometheus serves /metrics on the admin listener for scraping, next to
	// whatever OTEL_METRICS_EXPORTER pushes.
	Prometheus bool
	LogLevel   slog.Level
	LogFormat  string
}

// Defaults.
const (
	DefaultListenAddr       = "127.0.0.1:11435"
	DefaultAdminAddr        = "127.0.0.1:11436"
	DefaultOllamaURL        = "http://127.0.0.1:11434"
	DefaultPollInterval     = 15 * time.Second
	DefaultShutdownTimeout  = 30 * time.Second
	DefaultMaxInspectBytes  = 32 << 20
	DefaultCardinalityLimit = 2000
)

// Load parses args (without the program name). getenv supplies the
// OLLAMETRY_* defaults; flags given explicitly win over the environment.
func Load(args []string, getenv func(string) string, usage io.Writer) (Config, error) {
	env := func(key, fallback string) string {
		if v := getenv("OLLAMETRY_" + key); v != "" {
			return v
		}
		return fallback
	}
	fs := flag.NewFlagSet("ollametry", flag.ContinueOnError)
	fs.SetOutput(usage)

	listen := fs.String("listen", env("LISTEN_ADDR", DefaultListenAddr), "proxy listen address [OLLAMETRY_LISTEN_ADDR]")
	admin := fs.String("admin-listen", env("ADMIN_ADDR", DefaultAdminAddr), "health endpoint listen address [OLLAMETRY_ADMIN_ADDR]")
	ollamaURL := fs.String("ollama-url", env("OLLAMA_URL", DefaultOllamaURL), "Ollama base URL [OLLAMETRY_OLLAMA_URL]")
	poll := fs.String("poll-interval", env("POLL_INTERVAL", DefaultPollInterval.String()), "how often to poll /api/ps and /api/tags [OLLAMETRY_POLL_INTERVAL]")
	shutdown := fs.String("shutdown-timeout", env("SHUTDOWN_TIMEOUT", DefaultShutdownTimeout.String()), "how long to let in-flight requests finish on shutdown [OLLAMETRY_SHUTDOWN_TIMEOUT]")
	inspect := fs.String("max-inspect-bytes", env("MAX_INSPECT_BYTES", strconv.Itoa(DefaultMaxInspectBytes)), "largest request body or response document decoded for metrics [OLLAMETRY_MAX_INSPECT_BYTES]")
	cardinality := fs.String("cardinality-limit", env("CARDINALITY_LIMIT", strconv.Itoa(DefaultCardinalityLimit)), "maximum attribute sets per metric [OLLAMETRY_CARDINALITY_LIMIT]")
	prometheus := fs.String("prometheus", env("PROMETHEUS", "true"), "serve Prometheus metrics at /metrics on the admin listener [OLLAMETRY_PROMETHEUS]")
	level := fs.String("log-level", env("LOG_LEVEL", "info"), "debug, info, warn or error [OLLAMETRY_LOG_LEVEL]")
	format := fs.String("log-format", env("LOG_FORMAT", "json"), "json or text [OLLAMETRY_LOG_FORMAT]")

	if err := fs.Parse(args); err != nil {
		return Config{}, fmt.Errorf("parse flags: %w", err)
	}
	if fs.NArg() > 0 {
		return Config{}, fmt.Errorf("unexpected arguments: %q", fs.Args())
	}

	var (
		cfg  = Config{ListenAddr: *listen, AdminAddr: *admin, LogFormat: strings.ToLower(*format)}
		errs []error
		err  error
	)
	check := func(name string, err error) {
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
		}
	}
	check("listen", validateAddr(cfg.ListenAddr))
	check("admin-listen", validateAddr(cfg.AdminAddr))
	cfg.OllamaURL, err = parseOllamaURL(*ollamaURL)
	check("ollama-url", err)
	cfg.PollInterval, err = positiveDuration(*poll)
	check("poll-interval", err)
	cfg.ShutdownTimeout, err = positiveDuration(*shutdown)
	check("shutdown-timeout", err)
	cfg.MaxInspectBytes, err = positiveInt(*inspect)
	check("max-inspect-bytes", err)
	cfg.CardinalityLimit, err = positiveInt(*cardinality)
	check("cardinality-limit", err)
	cfg.Prometheus, err = strconv.ParseBool(*prometheus)
	check("prometheus", err)
	check("log-level", cfg.LogLevel.UnmarshalText([]byte(*level)))
	if cfg.LogFormat != "json" && cfg.LogFormat != "text" {
		check("log-format", fmt.Errorf("%q is not json or text", *format))
	}
	if cfg.ListenAddr == cfg.AdminAddr {
		check("admin-listen", fmt.Errorf("%q is also the proxy listen address", cfg.AdminAddr))
	}
	if len(errs) > 0 {
		return Config{}, fmt.Errorf("invalid configuration: %w", errors.Join(errs...))
	}
	return cfg, nil
}

func validateAddr(addr string) error {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("%q is not host:port: %w", addr, err)
	}
	if _, err := strconv.ParseUint(port, 10, 16); err != nil {
		return fmt.Errorf("%q has an invalid port: %w", addr, err)
	}
	return nil
}

func parseOllamaURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse %q: %w", raw, err)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("%q must be an absolute http or https URL", raw)
	}
	return u, nil
}

func positiveDuration(raw string) (time.Duration, error) {
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("parse duration %q: %w", raw, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("%s must be positive", d)
	}
	return d, nil
}

func positiveInt(raw string) (int, error) {
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("parse integer %q: %w", raw, err)
	}
	if n <= 0 {
		return 0, fmt.Errorf("%d must be positive", n)
	}
	return n, nil
}
