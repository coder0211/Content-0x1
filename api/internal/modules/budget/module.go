// Package budget wires the budget module: pricing/estimates, reservations,
// usage records, alerts and the usage/budget endpoints (SPEC-02 §6).
package budget

import (
	"context"
	"log/slog"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/be/acs/api/internal/app/registry"
	nethttp "github.com/be/acs/api/internal/modules/budget/adapter/http"
	"github.com/be/acs/api/internal/modules/budget/adapter/postgres"
	"github.com/be/acs/api/internal/modules/budget/usecase"
)

// AlertMailInterval is how often the worker flushes the alert e-mail outbox.
const AlertMailInterval = 30 * time.Second

// Module is the budget module.
type Module struct {
	svc     *usecase.Service
	handler *nethttp.Handler
}

// Name implements registry.Module.
func (*Module) Name() string { return "budget" }

// Routes implements registry.HTTPModule.
func (m *Module) Routes(r chi.Router) { m.handler.Routes(r) }

// Runners implements registry.RunnerModule: the worker delivers queued
// budget alert e-mails (written in the same transaction as the alert).
func (m *Module) Runners(p registry.Process) []registry.Runner {
	if p != registry.ProcessWorker || m.svc.Mailer == nil {
		return nil
	}
	return []registry.Runner{{Name: "budget-alert-mail", Run: func(ctx context.Context) error {
		t := time.NewTicker(AlertMailInterval)
		defer t.Stop()
		for {
			if _, err := m.svc.DeliverAlertEmails(ctx); err != nil && ctx.Err() == nil {
				m.svc.Logger.WarnContext(ctx, "budget alert mail delivery failed", slog.String("error", err.Error()))
			}
			select {
			case <-ctx.Done():
				return nil
			case <-t.C:
			}
		}
	}}}
}

// Register publishes APIs.Budget.
func Register(c *registry.Container) (registry.Module, error) {
	logger := c.Logger
	if logger == nil {
		logger = slog.Default()
	}
	svc := &usecase.Service{
		Tx: c.Tx, Repo: postgres.NewRepository(c.DB), Clock: c.Clock, Mailer: c.Mailer, Logger: logger,
	}
	if c.APIs.Audit != nil {
		svc.Audit, svc.Notifier = c.APIs.Audit, c.APIs.Audit
	}
	if c.APIs.Identity != nil {
		svc.Users = c.APIs.Identity
	}
	c.APIs.Budget = svc
	return &Module{svc: svc, handler: &nethttp.Handler{Svc: svc}}, nil
}
