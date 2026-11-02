package ollama

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// neverExpires is how far ahead an expiry must be to mean "kept loaded
// forever" (keep_alive < 0), which Ollama reports as a date centuries away.
const neverExpires = 365 * 24 * time.Hour

// Snapshot is the result of one poll.
type Snapshot struct {
	At        time.Time
	Up        bool
	Version   string
	Running   []RunningModel
	Installed []InstalledModel
	// Partial is set when the server answered /api/version but /api/ps or
	// /api/tags failed; the failed list is nil.
	Partial bool
}

// Poller periodically snapshots Ollama's state.
type Poller struct {
	client   *Client
	interval time.Duration
	timeout  time.Duration
	logger   *slog.Logger
	now      func() time.Time

	mu   sync.RWMutex
	last Snapshot
}

// NewPoller returns a Poller; call Run to start it.
func NewPoller(client *Client, interval time.Duration, logger *slog.Logger) *Poller {
	return &Poller{
		client:   client,
		interval: interval,
		timeout:  min(interval, 10*time.Second),
		logger:   logger,
		now:      time.Now,
	}
}

// Run polls immediately and then every interval until ctx is done.
func (p *Poller) Run(ctx context.Context) {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	for {
		p.Poll(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Poll takes one snapshot and stores it. A poll cut short because parent
// was canceled stores nothing, so shutdown never reports Ollama as down.
func (p *Poller) Poll(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, p.timeout)
	defer cancel()

	s := Snapshot{At: p.now()}
	version, err := p.client.Version(ctx)
	if err != nil {
		if parent.Err() != nil {
			return // shutting down: the failure says nothing about Ollama
		}
		p.logChange(ctx, slog.LevelWarn, "ollama unreachable", err)
		p.store(s)
		return
	}
	s.Up, s.Version = true, version

	var errs []error
	if s.Running, err = p.client.Running(ctx); err != nil {
		errs = append(errs, err)
	}
	if s.Installed, err = p.client.Installed(ctx); err != nil {
		errs = append(errs, err)
	}
	if len(errs) > 0 && parent.Err() != nil {
		return
	}
	if len(errs) > 0 {
		s.Partial = true
		p.logChange(ctx, slog.LevelWarn, "ollama poll incomplete", errors.Join(errs...))
	} else {
		p.logChange(ctx, slog.LevelInfo, "ollama reachable", nil)
	}
	p.store(s)
}

// logChange logs only transitions between up, partial and down, so a
// lasting outage does not log every interval.
func (p *Poller) logChange(ctx context.Context, level slog.Level, msg string, err error) {
	prev := p.Snapshot()
	wasHealthy := prev.Up && !prev.Partial
	healthy := err == nil
	if !prev.At.IsZero() && wasHealthy == healthy {
		return
	}
	attrs := []slog.Attr{}
	if err != nil {
		attrs = append(attrs, slog.Any("error", err))
	}
	p.logger.LogAttrs(ctx, level, msg, attrs...)
}

func (p *Poller) store(s Snapshot) {
	p.mu.Lock()
	p.last = s
	p.mu.Unlock()
}

// Snapshot returns the latest snapshot.
func (p *Poller) Snapshot() Snapshot {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.last
}

// Ready reports whether the last poll reached Ollama and is recent.
func (p *Poller) Ready() bool {
	s := p.Snapshot()
	return s.Up && p.now().Sub(s.At) <= 3*p.interval
}

// KnownModel reports whether name is installed or loaded, comparing names the
// way Ollama resolves them (case-insensitive, ":latest" when no tag is given).
// Before the first successful /api/tags it reports true, so a missing
// inventory never hides real models.
func (p *Poller) KnownModel(name string) bool {
	s := p.Snapshot()
	if s.Installed == nil {
		return true
	}
	want := normalizeModelName(name)
	for _, m := range s.Installed {
		if normalizeModelName(m.Name) == want {
			return true
		}
	}
	for _, m := range s.Running {
		if normalizeModelName(m.Name) == want {
			return true
		}
	}
	return false
}

func normalizeModelName(name string) string {
	name = strings.ToLower(name)
	if !strings.Contains(name[strings.LastIndexByte(name, '/')+1:], ":") {
		name += ":latest"
	}
	return name
}

// RegisterMetrics registers observable gauges fed from the latest snapshot.
func (p *Poller) RegisterMetrics(meter metric.Meter) (metric.Registration, error) {
	var errs []error
	gauge := func(name, unit, desc string) metric.Int64ObservableGauge {
		g, err := meter.Int64ObservableGauge(name, metric.WithUnit(unit), metric.WithDescription(desc))
		errs = append(errs, err)
		return g
	}
	var (
		up            = gauge("ollama.up", "", "Whether the last poll reached Ollama (1) or not (0).")
		buildInfo     = gauge("ollama.build.info", "", "Always 1 while Ollama is reachable; carries ollama.version.")
		memSize       = gauge("ollama.model.memory.size", "By", "Memory held by a loaded model.")
		vramSize      = gauge("ollama.model.memory.vram.size", "By", "Part of a loaded model's memory held in GPU memory.")
		contextLength = gauge("ollama.model.context_length", "{token}", "Context window a loaded model was started with.")
		keepAlive     = gauge("ollama.model.keep_alive.remaining", "s", "Seconds until an idle loaded model is unloaded; absent when it is kept forever.")
		installed     = gauge("ollama.models.installed", "{model}", "Models installed locally.")
		storage       = gauge("ollama.models.storage.size", "By", "Disk space used by locally installed models.")
	)
	if err := errors.Join(errs...); err != nil {
		return nil, fmt.Errorf("create ollama gauges: %w", err)
	}

	reg, err := meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		s := p.Snapshot()
		if s.At.IsZero() {
			return nil // no poll yet: report nothing rather than a false "down"
		}
		if !s.Up {
			o.ObserveInt64(up, 0)
			return nil
		}
		// up has no attributes so that up and down are one series: a version label
		// would leave the last "up" series lingering in Prometheus after a stop.
		o.ObserveInt64(up, 1)
		o.ObserveInt64(buildInfo, 1, metric.WithAttributes(attribute.String("ollama.version", s.Version)))

		now := p.now()
		for _, m := range s.Running {
			attrs := metric.WithAttributes(runningAttrs(m)...)
			o.ObserveInt64(memSize, m.Size, attrs)
			o.ObserveInt64(vramSize, m.SizeVRAM, attrs)
			if m.ContextLength > 0 {
				o.ObserveInt64(contextLength, m.ContextLength, attrs)
			}
			if left := m.ExpiresAt.Sub(now); !m.ExpiresAt.IsZero() && left < neverExpires {
				o.ObserveInt64(keepAlive, int64(max(left, 0).Seconds()), attrs)
			}
		}
		if s.Installed != nil {
			var total int64
			local := 0
			for _, m := range s.Installed {
				if m.RemoteHost != "" {
					continue // cloud models take no local disk
				}
				local++
				total += m.Size
			}
			o.ObserveInt64(installed, int64(local))
			o.ObserveInt64(storage, total)
		}
		return nil
	}, up, buildInfo, memSize, vramSize, contextLength, keepAlive, installed, storage)
	if err != nil {
		return nil, fmt.Errorf("register ollama gauge callback: %w", err)
	}
	return reg, nil
}

func runningAttrs(m RunningModel) []attribute.KeyValue {
	runner := m.Runner
	if runner == "" {
		runner = m.Details.Runner
	}
	return []attribute.KeyValue{
		attribute.String("ollama.model.name", m.Name),
		attribute.String("ollama.model.family", m.Details.Family),
		attribute.String("ollama.model.parameter_size", m.Details.ParameterSize),
		attribute.String("ollama.model.quantization", m.Details.QuantizationLevel),
		attribute.String("ollama.model.runner", runner),
	}
}
