package usecase_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/be/acs/api/internal/modules/identity/domain"
	"github.com/be/acs/api/internal/modules/identity/identitytest"
	"github.com/be/acs/api/internal/modules/identity/usecase"
	"github.com/be/acs/api/internal/shared/kernel"
	"github.com/be/acs/api/internal/testutil"
)

type fixture struct {
	auth  *usecase.Auth
	users *usecase.Users
	clock *testutil.Clock
	audit *testutil.Audit
	sess  *identitytest.Sessions
}

func newFixture() *fixture {
	ur, sr, clk, au, tx := identitytest.NewUsers(), identitytest.NewSessions(), testutil.NewClock(), &testutil.Audit{}, &testutil.Tx{}
	return &fixture{
		auth: &usecase.Auth{Tx: tx, Users: ur, Sessions: sr, Hasher: identitytest.PlainHasher{}, Clock: clk, Audit: au,
			Lockout: domain.LockoutPolicy{MaxFailed: 5, Window: 15 * time.Minute},
			Session: domain.SessionPolicy{Idle: 12 * time.Hour, Absolute: 7 * 24 * time.Hour}},
		users: &usecase.Users{Tx: tx, Users: ur, Sessions: sr, Hasher: identitytest.PlainHasher{}, Clock: clk, Audit: au},
		clock: clk, audit: au, sess: sr,
	}
}

func (f *fixture) seed(t *testing.T, email string, roles ...string) *domain.User {
	t.Helper()
	u, err := f.users.Bootstrap(context.Background(), usecase.CreateUserInput{Email: email, Name: "User", RoleCodes: roles, TempPassword: "correct-horse"})
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestLoginAndAuthenticate(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	f.seed(t, "creator@acs.local", kernel.RoleContentCreator)
	out, err := f.auth.Login(ctx, usecase.LoginInput{Email: "Creator@acs.local", Password: "correct-horse"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Token == "" || out.CSRFToken == "" || !slices.Contains(out.User.Permissions, kernel.PermGenerateRun) {
		t.Fatalf("%+v", out)
	}
	actor, sess, err := f.auth.Authenticate(ctx, out.Token)
	if err != nil || actor.UserID != out.User.ID || sess.CSRFToken != out.CSRFToken {
		t.Fatalf("authenticate: %v %+v", err, actor)
	}
	if _, _, err := f.auth.Authenticate(ctx, "garbage"); !errors.Is(err, kernel.ErrUnauthenticated) {
		t.Fatalf("got %v", err)
	}
	f.clock.Advance(13 * time.Hour)
	if _, _, err := f.auth.Authenticate(ctx, out.Token); !errors.Is(err, kernel.ErrSessionExpired) {
		t.Fatalf("idle timeout: %v", err)
	}
}

func TestLoginLockout(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	f.seed(t, "a@acs.local", kernel.RoleReviewer)
	for i := 0; i < 4; i++ {
		if _, err := f.auth.Login(ctx, usecase.LoginInput{Email: "a@acs.local", Password: "wrong-password"}); !errors.Is(err, kernel.ErrInvalidCredentials) {
			t.Fatalf("attempt %d: %v", i, err)
		}
	}
	if _, err := f.auth.Login(ctx, usecase.LoginInput{Email: "a@acs.local", Password: "wrong-password"}); !errors.Is(err, kernel.ErrAccountLocked) {
		t.Fatalf("5th: %v", err)
	}
	if _, err := f.auth.Login(ctx, usecase.LoginInput{Email: "a@acs.local", Password: "correct-horse"}); !errors.Is(err, kernel.ErrAccountLocked) {
		t.Fatalf("locked user must not log in: %v", err)
	}
	if !slices.Contains(f.audit.Actions(), "user.lockout") {
		t.Fatal("lockout not audited")
	}
	f.clock.Advance(16 * time.Minute)
	if _, err := f.auth.Login(ctx, usecase.LoginInput{Email: "a@acs.local", Password: "correct-horse"}); err != nil {
		t.Fatalf("after lockout: %v", err)
	}
}

func TestUnknownEmailIsInvalidCredentials(t *testing.T) {
	f := newFixture()
	if _, err := f.auth.Login(context.Background(), usecase.LoginInput{Email: "ghost@acs.local", Password: "x"}); !errors.Is(err, kernel.ErrInvalidCredentials) {
		t.Fatal(err)
	}
}

func TestLockUserRevokesSessions(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	admin := testutil.Actor(kernel.RoleAdmin)
	u := f.seed(t, "c@acs.local", kernel.RoleContentCreator)
	out, err := f.auth.Login(ctx, usecase.LoginInput{Email: "c@acs.local", Password: "correct-horse"})
	if err != nil {
		t.Fatal(err)
	}
	locked := string(domain.StatusLocked)
	if _, err := f.users.Update(ctx, admin, u.ID, usecase.UpdateUserInput{Status: &locked}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.auth.Authenticate(ctx, out.Token); !errors.Is(err, kernel.ErrSessionExpired) {
		t.Fatalf("locked user's next request must be 401: %v", err)
	}
	if !slices.Contains(f.audit.Actions(), "user.update") {
		t.Fatal("not audited")
	}
}

func TestUserManagementRBAC(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	for _, role := range kernel.AllRoles {
		actor := testutil.Actor(role)
		_, err := f.users.Create(ctx, actor, usecase.CreateUserInput{Email: role + "@x.vn", Name: "N", RoleCodes: []string{kernel.RoleReviewer}, TempPassword: "long-enough-pw"})
		if (role == kernel.RoleAdmin) != (err == nil) {
			t.Errorf("%s create: %v", role, err)
		}
	}
}

func TestRoleChangeAuditedAndSelfLockForbidden(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	adminUser := f.seed(t, "admin@acs.local", kernel.RoleAdmin)
	admin := testutil.Actor(kernel.RoleAdmin)
	admin.UserID = adminUser.ID
	u := f.seed(t, "r@acs.local", kernel.RoleReviewer)
	roles := []string{kernel.RoleMarketingLead}
	if _, err := f.users.Update(ctx, admin, u.ID, usecase.UpdateUserInput{RoleCodes: &roles}); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(f.audit.Actions(), "role.assign") {
		t.Fatal("role.assign not audited")
	}
	disabled := string(domain.StatusDisabled)
	if _, err := f.users.Update(ctx, admin, adminUser.ID, usecase.UpdateUserInput{Status: &disabled}); !errors.Is(err, kernel.ErrForbidden) {
		t.Fatalf("self-disable: %v", err)
	}
}
