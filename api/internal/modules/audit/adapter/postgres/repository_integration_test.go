package postgres_test

import (
	"context"
	"testing"

	"github.com/be/acs/api/internal/modules/audit/adapter/postgres"
	"github.com/be/acs/api/internal/modules/audit/usecase"
	"github.com/be/acs/api/internal/shared/kernel"
	"github.com/be/acs/api/internal/testutil/pgtest"
)

// ACS-009 AC: UPDATE/DELETE on audit_logs fails.
func TestAuditLogsAreAppendOnly(t *testing.T) {
	pool := pgtest.Postgres(t)
	ctx := context.Background()
	svc := &usecase.Service{Repo: postgres.NewRepository(pool)}
	ctx = kernel.WithRequestMeta(ctx, kernel.RequestMeta{RequestID: "r1", IP: "10.1.2.3"})
	if err := svc.Record(ctx, "budget.update", "budget", "global", nil, map[string]int{"limit": 1000}); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE audit_logs SET action = 'x'`); err == nil {
		t.Fatal("UPDATE audit_logs must fail")
	}
	if _, err := pool.Exec(ctx, `DELETE FROM audit_logs`); err == nil {
		t.Fatal("DELETE audit_logs must fail")
	}
	p, err := svc.ListLogs(ctx, kernel.Actor{UserID: kernel.NewID(), Permissions: []string{kernel.PermAuditView}}, usecase.LogFilter{Action: "budget.update"}, kernel.PageRequest{})
	if err != nil || len(p.Items) != 1 || p.Items[0].IP != "10.1.2.3" {
		t.Fatalf("%v %+v", err, p)
	}
}
