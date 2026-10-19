package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/be/acs/api/internal/app/registry"
	"github.com/be/acs/api/internal/platform/config"
	"github.com/be/acs/api/internal/platform/ffmpeg"
	applog "github.com/be/acs/api/internal/platform/log"
	"github.com/be/acs/api/internal/platform/mail"
	"github.com/be/acs/api/internal/platform/metrics"
	"github.com/be/acs/api/internal/platform/postgres"
	"github.com/be/acs/api/internal/platform/queue"
	"github.com/be/acs/api/internal/platform/redis"
	"github.com/be/acs/api/internal/platform/s3"
	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/providers"
)

// App is a composed process.
type App struct {
	Container *registry.Container
	Modules   []registry.Module
	closers   []func()
}

// Close releases platform resources in reverse order.
func (a *App) Close() {
	for i := len(a.closers) - 1; i >= 0; i-- {
		a.closers[i]()
	}
}

// Options select which drivers a process needs.
type Options struct {
	Process   registry.Process
	NeedRedis bool
}

// New loads drivers, then constructs every module in Modules order.
func New(ctx context.Context, cfg config.Config, opts Options) (*App, error) {
	logger := applog.New(cfg.LogLevel).With("process", string(opts.Process))
	slog.SetDefault(logger)
	a := &App{}
	fail := func(err error) (*App, error) {
		a.Close()
		return nil, err
	}

	pool, err := postgres.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return fail(err)
	}
	a.closers = append(a.closers, pool.Close)

	store, err := s3.New(cfg.S3, cfg.SignedURLTTL)
	if err != nil {
		return fail(fmt.Errorf("s3: %w", err))
	}

	p := registry.Platform{
		Process: opts.Process, Config: cfg, Logger: logger, DB: pool,
		Tx: postgres.NewTxManager(pool), Clock: ports.SystemClock{}, Storage: store,
		Prober: ffmpeg.NewProber(cfg.FFmpeg.FFprobePath), Renderer: ffmpeg.NewRenderer(cfg.FFmpeg.FFmpegPath),
		AI: providers.Build(cfg), Metrics: metrics.New(), Mailer: mail.New(cfg.SMTP),
	}
	if opts.NeedRedis {
		rc, err := redis.Connect(ctx, cfg.RedisURL)
		if err != nil {
			return fail(err)
		}
		a.closers = append(a.closers, func() { _ = rc.Close() })
		ev := redis.NewEvents(rc)
		p.Redis, p.Events, p.Subscriber = rc, ev, ev
		qc, err := queue.NewClient(cfg.RedisURL)
		if err != nil {
			return fail(err)
		}
		a.closers = append(a.closers, func() { _ = qc.Close() })
		p.Queue = qc
	}

	c := &registry.Container{Platform: p}
	mods, err := Compose(c, Modules)
	if err != nil {
		return fail(err)
	}
	a.Container, a.Modules = c, mods
	wireHooks(c)
	return a, nil
}

// Compose runs constructors in order. Exported for tests that compose with
// fake platforms.
func Compose(c *registry.Container, ctors []registry.Constructor) ([]registry.Module, error) {
	mods := make([]registry.Module, 0, len(ctors))
	seen := map[string]bool{}
	for _, ctor := range ctors {
		m, err := ctor(c)
		if err != nil {
			return nil, err
		}
		if seen[m.Name()] {
			return nil, fmt.Errorf("module %q registered twice", m.Name())
		}
		seen[m.Name()] = true
		mods = append(mods, m)
	}
	return mods, nil
}

// RegisterSteps hands every module's step handlers to the job module.
func RegisterSteps(a *App) error {
	if a.Container.APIs.Steps == nil {
		return errors.New("job module did not publish a step registrar")
	}
	for _, m := range a.Modules {
		if sm, ok := m.(registry.StepModule); ok {
			for _, h := range sm.StepHandlers() {
				if err := a.Container.APIs.Steps.Register(h); err != nil {
					return fmt.Errorf("%s: %w", m.Name(), err)
				}
			}
		}
	}
	return nil
}

// Runners collects background runners for the process.
func Runners(a *App) []registry.Runner {
	var out []registry.Runner
	for _, m := range a.Modules {
		if rm, ok := m.(registry.RunnerModule); ok {
			out = append(out, rm.Runners(a.Container.Process)...)
		}
	}
	return out
}
