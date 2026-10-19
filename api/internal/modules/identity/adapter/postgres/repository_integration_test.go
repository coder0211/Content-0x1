package postgres_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/be/acs/api/internal/modules/identity/adapter/postgres"
	"github.com/be/acs/api/internal/modules/identity/domain"
	"github.com/be/acs/api/internal/modules/identity/usecase"
	db "github.com/be/acs/api/internal/platform/postgres"
	"github.com/be/acs/api/internal/shared/kernel"
	"github.com/be/acs/api/internal/testutil/pgtest"
)

func TestUserAndSessionRepositories(t *testing.T) {
	pool := pgtest.Postgres(t)
	ctx := context.Background()
	users, sessions, tx := postgres.NewUserRepository(pool), postgres.NewSessionRepository(pool), db.NewTxManager(pool)
	now := time.Now().UTC().Truncate(time.Microsecond)
	u, _ := domain.NewUser("Lead@ACS.local", "Lead", "hash", []string{kernel.RoleMarketingLead, kernel.RoleReviewer}, now)
	err := tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := users.Create(ctx, u); err != nil {
			return err
		}
		return users.SetRoles(ctx, u.ID, u.Roles)
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := users.Create(ctx, u); !errors.Is(err, kernel.ErrConflict) {
		t.Fatalf("duplicate email: %v", err)
	}
	got, err := users.GetByEmail(ctx, "lead@acs.local")
	if err != nil || !slices.Equal(got.Roles, []string{kernel.RoleMarketingLead, kernel.RoleReviewer}) {
		t.Fatalf("%v %+v", err, got)
	}
	perms, _ := users.Permissions(ctx, u.ID)
	if !slices.Contains(perms, kernel.PermBudgetManage) || slices.Contains(perms, kernel.PermUserManage) {
		t.Fatalf("perms %v", perms)
	}
	if err := users.SetRoles(ctx, u.ID, []string{"ghost"}); !errors.Is(err, kernel.ErrValidation) {
		t.Fatalf("unknown role: %v", err)
	}
	page, err := users.List(ctx, usecase.UserFilter{Query: "lead"}, kernel.PageRequest{Limit: 10})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("%v %+v", err, page)
	}
	s := domain.NewSession(u.ID, usecase.HashToken("tok"), "csrf", "127.0.0.1", "test", now, domain.SessionPolicy{Idle: time.Hour, Absolute: 24 * time.Hour})
	if err := sessions.Create(ctx, s); err != nil {
		t.Fatal(err)
	}
	if err := sessions.RevokeAllForUser(ctx, u.ID, now); err != nil {
		t.Fatal(err)
	}
	gs, err := sessions.GetByTokenHash(ctx, usecase.HashToken("tok"))
	if err != nil || gs.RevokedAt == nil || gs.IP != "127.0.0.1" {
		t.Fatalf("%v %+v", err, gs)
	}
}

// RBAC seed (migration 0011) must equal kernel.RolePermissions.
func TestSeededRBACMatchesKernel(t *testing.T) {
	pool := pgtest.Postgres(t)
	rows, err := pool.Query(context.Background(), `SELECT r.code, rp.permission FROM role_permissions rp JOIN roles r ON r.id = rp.role_id`)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string][]string{}
	for rows.Next() {
		var role, perm string
		if err := rows.Scan(&role, &perm); err != nil {
			t.Fatal(err)
		}
		got[role] = append(got[role], perm)
	}
	for role, want := range kernel.RolePermissions {
		g := slices.Clone(got[role])
		w := slices.Clone(want)
		slices.Sort(g)
		slices.Sort(w)
		if !slices.Equal(g, w) {
			t.Errorf("%s: db %v kernel %v", role, g, w)
		}
	}
}
