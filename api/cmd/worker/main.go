// Command worker runs job step handlers and background loops for the queues
// in WORKER_QUEUES (or --queues=text,image,...). Scale per queue by running
// several workers with different queue sets (SPEC-02 §5.4).
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"golang.org/x/sync/errgroup"

	"github.com/be/acs/api/internal/app"
	"github.com/be/acs/api/internal/app/registry"
	"github.com/be/acs/api/internal/platform/config"
)

func main() {
	if err := run(); err != nil {
		slog.Error("worker exited", "error", err)
		os.Exit(1)
	}
}

func run() error {
	queues := flag.String("queues", "", "comma-separated queues to consume (default WORKER_QUEUES)")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if *queues != "" {
		cfg.Worker.Queues = strings.Split(*queues, ",")
	}
	a, err := app.New(ctx, cfg, app.Options{Process: registry.ProcessWorker, NeedRedis: true})
	if err != nil {
		return err
	}
	defer a.Close()
	if err := app.RegisterSteps(a); err != nil {
		return err
	}
	runners := app.Runners(a)
	a.Container.Logger.Info("worker started", "queues", cfg.Worker.Queues, "runners", len(runners))
	if len(runners) == 0 {
		a.Container.Logger.Warn("no background runners registered (job engine not implemented yet)")
	}
	g, gctx := errgroup.WithContext(ctx)
	for _, r := range runners {
		g.Go(func() error {
			a.Container.Logger.Info("runner started", "runner", r.Name)
			return r.Run(gctx)
		})
	}
	g.Go(func() error { <-gctx.Done(); return nil })
	return g.Wait()
}
