// Command ollametry is a transparent reverse proxy for Ollama that exports
// OpenTelemetry metrics: GenAI token usage, latency and time to first token
// per model, HTTP error rates, and the models Ollama holds in memory.
//
// Usage:
//
//	ollametry [flags]       run the proxy (see -help)
//	ollametry healthcheck   probe /livez on the admin address; for container health checks
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"

	"github.com/a1exus/ollametry/internal/admin"
	"github.com/a1exus/ollametry/internal/config"
	"github.com/a1exus/ollametry/internal/ollama"
	"github.com/a1exus/ollametry/internal/proxy"
	"github.com/a1exus/ollametry/internal/telemetry"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(healthcheck(os.Getenv("OLLAMETRY_ADMIN_ADDR")))
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, os.Args[1:], os.Getenv, os.Stdout, os.Stderr)
	stop()
	switch {
	case errors.Is(err, flag.ErrHelp):
		os.Exit(0)
	case err != nil:
		fmt.Fprintf(os.Stderr, "ollametry: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) error {
	cfg, err := config.Load(args, getenv, stderr)
	if err != nil {
		return err
	}
	logger := newLogger(cfg, stdout)

	var (
		extraReaders   []sdkmetric.Reader
		metricsHandler http.Handler
	)
	if cfg.Prometheus {
		reader, handler, err := telemetry.NewPrometheusReader()
		if err != nil {
			return fmt.Errorf("set up Prometheus endpoint: %w", err)
		}
		extraReaders, metricsHandler = append(extraReaders, reader), handler
	}
	mp, err := telemetry.NewMeterProvider(ctx, telemetry.ProviderConfig{
		ServiceVersion:   version,
		CardinalityLimit: cfg.CardinalityLimit,
		ExtraReaders:     extraReaders,
	})
	if err != nil {
		return fmt.Errorf("set up metrics: %w", err)
	}
	defer func() {
		// Flush the final export even though ctx is already canceled.
		flushCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		if err := mp.Shutdown(flushCtx); err != nil {
			logger.Error("flush metrics on shutdown", slog.Any("error", err))
		}
	}()
	meter := mp.Meter(telemetry.MeterName)

	instruments, err := telemetry.NewInstruments(meter)
	if err != nil {
		return err
	}
	poller := ollama.NewPoller(
		ollama.NewClient(cfg.OllamaURL, &http.Client{Transport: proxy.NewTransport()}),
		cfg.PollInterval, logger)
	p, err := proxy.New(proxy.Config{
		Upstream:        cfg.OllamaURL,
		MaxInspectBytes: cfg.MaxInspectBytes,
		Instruments:     instruments,
		Logger:          logger,
		KnownModel:      poller.KnownModel,
	})
	if err != nil {
		return err
	}
	if _, err := poller.RegisterMetrics(meter); err != nil {
		return err
	}
	pollCtx, stopPolling := context.WithCancel(ctx)
	defer stopPolling()
	go poller.Run(pollCtx)

	var shuttingDown atomic.Bool
	ready := func() bool { return !shuttingDown.Load() && poller.Ready() }

	proxySrv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           p,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		// No read or write timeout: a generation can stream for many minutes.
		ErrorLog: slog.NewLogLogger(logger.Handler(), slog.LevelDebug),
	}
	adminSrv := &http.Server{
		Addr:              cfg.AdminAddr,
		Handler:           admin.Handler(ready, metricsHandler),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Second,
	}

	// Bind both listeners before announcing readiness so a port clash fails
	// startup instead of surfacing later.
	var lc net.ListenConfig
	proxyLn, err := lc.Listen(ctx, "tcp", cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("listen for proxy on %s: %w", cfg.ListenAddr, err)
	}
	adminLn, err := lc.Listen(ctx, "tcp", cfg.AdminAddr)
	if err != nil {
		_ = proxyLn.Close()
		return fmt.Errorf("listen for admin on %s: %w", cfg.AdminAddr, err)
	}

	serveErr := make(chan error, 2)
	go func() { serveErr <- serve(proxySrv, proxyLn, "proxy") }()
	go func() { serveErr <- serve(adminSrv, adminLn, "admin") }()

	logger.Info("ollametry started",
		slog.String("version", version),
		slog.String("listen", cfg.ListenAddr),
		slog.String("admin_listen", cfg.AdminAddr),
		slog.String("ollama_url", cfg.OllamaURL.String()))

	var runErr error
	select {
	case <-ctx.Done():
		logger.Info("shutting down", slog.Duration("timeout", cfg.ShutdownTimeout))
	case runErr = <-serveErr:
		logger.Error("server failed, shutting down", slog.Any("error", runErr))
	}

	shuttingDown.Store(true)
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.ShutdownTimeout)
	defer cancel()
	if err := proxySrv.Shutdown(shutdownCtx); err != nil {
		// In-flight streams outlived the timeout; cut them off.
		logger.Warn("proxy did not drain in time, closing connections", slog.Any("error", err))
		_ = proxySrv.Close()
	}
	// Close does not wait for handlers. Closing their connections cancels
	// their upstream requests, so they finish quickly; their metrics must land
	// before the final flush.
	waitCtx, cancelWait := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancelWait()
	if err := p.Wait(waitCtx); err != nil {
		logger.Warn("handlers still running, their metrics may be lost", slog.Any("error", err))
	}
	if err := adminSrv.Shutdown(shutdownCtx); err != nil {
		_ = adminSrv.Close()
	}
	stopPolling()
	logger.Info("ollametry stopped")
	return runErr
}

func serve(srv *http.Server, ln net.Listener, name string) error {
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("%s server on %s: %w", name, ln.Addr(), err)
	}
	return nil
}

func newLogger(cfg config.Config, w io.Writer) *slog.Logger {
	opts := &slog.HandlerOptions{Level: cfg.LogLevel}
	if cfg.LogFormat == "text" {
		return slog.New(slog.NewTextHandler(w, opts))
	}
	return slog.New(slog.NewJSONHandler(w, opts))
}

// healthcheck probes the admin /livez endpoint and returns an exit code. The
// distroless image has no shell or curl, so the binary checks itself.
func healthcheck(adminAddr string) int {
	if adminAddr == "" {
		adminAddr = config.DefaultAdminAddr
	}
	_, port, err := net.SplitHostPort(adminAddr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck: invalid admin address %q: %v\n", adminAddr, err)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	// Always probe loopback: the listener may be bound to a wildcard address.
	url := "http://" + net.JoinHostPort("127.0.0.1", port) + "/livez"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil) //nolint:gosec // G704: loopback URL to this process's own admin listener
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck: build request for %s: %v\n", url, err)
		return 1
	}
	resp, err := http.DefaultClient.Do(req) //nolint:gosec // G704: see above
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck: GET %s: %v\n", url, err)
		return 1
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "healthcheck: GET %s: status %d, want 200\n", url, resp.StatusCode)
		return 1
	}
	return 0
}
