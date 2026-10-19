// Command server runs the HTTP API (REST /api/v1, SSE, health, metrics).
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/be/acs/api/internal/app"
	"github.com/be/acs/api/internal/app/registry"
	"github.com/be/acs/api/internal/modules/identity"
	"github.com/be/acs/api/internal/platform/config"
	"github.com/be/acs/api/internal/platform/httpx"
	"github.com/be/acs/api/internal/platform/s3"
)

func main() {
	if err := run(); err != nil {
		slog.Error("server exited", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	a, err := app.New(ctx, cfg, app.Options{Process: registry.ProcessServer, NeedRedis: true})
	if err != nil {
		return err
	}
	defer a.Close()
	c := a.Container

	readiness := []httpx.ReadinessCheck{
		{Name: "postgres", Check: func(ctx context.Context) error { return c.DB.Ping(ctx) }},
		{Name: "redis", Check: func(ctx context.Context) error { return c.Redis.Ping(ctx).Err() }},
	}
	if st, ok := c.Storage.(*s3.Storage); ok {
		readiness = append(readiness, httpx.ReadinessCheck{Name: "storage", Check: st.Ping})
	}
	root, api := httpx.NewRouter(httpx.Options{
		Logger: c.Logger, Metrics: c.Metrics, CORSOrigins: cfg.CORSOrigins,
		Authenticator: c.APIs.Authenticator, Cookies: identity.Cookies(c), Readiness: readiness,
	})
	for _, m := range a.Modules {
		if hm, ok := m.(registry.HTTPModule); ok {
			hm.Routes(api)
		}
	}

	srv := &http.Server{
		Addr: cfg.HTTPAddr, Handler: root,
		ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, IdleTimeout: 120 * time.Second,
		// WriteTimeout stays 0: SSE streams are long-lived; handlers bound their own work.
	}
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		c.Logger.Info("http listening", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	})
	for _, r := range app.Runners(a) {
		g.Go(func() error { return r.Run(gctx) })
	}
	g.Go(func() error {
		<-gctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		return srv.Shutdown(shutdown)
	})
	return g.Wait()
}
