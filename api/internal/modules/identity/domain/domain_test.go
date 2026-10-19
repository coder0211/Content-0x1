package domain

import (
	"errors"
	"testing"
	"time"

	"github.com/be/acs/api/internal/shared/kernel"
)

var policy = LockoutPolicy{MaxFailed: 5, Window: 15 * time.Minute}

func TestLockoutAfterFiveFailures(t *testing.T) {
	now := time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)
	u := &User{Status: StatusActive, PasswordHash: "h"}
	for i := 1; i <= 4; i++ {
		if u.RegisterFailedLogin(now.Add(time.Duration(i)*time.Minute), policy) {
			t.Fatalf("locked after %d failures", i)
		}
	}
	if !u.RegisterFailedLogin(now.Add(5*time.Minute), policy) {
		t.Fatal("should lock on 5th failure")
	}
	if err := u.CheckCanLogin(now.Add(10 * time.Minute)); !errors.Is(err, kernel.ErrAccountLocked) {
		t.Fatalf("got %v", err)
	}
	if err := u.CheckCanLogin(now.Add(21 * time.Minute)); err != nil {
		t.Fatalf("lock should expire: %v", err)
	}
}

func TestFailuresOutsideWindowReset(t *testing.T) {
	now := time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)
	u := &User{Status: StatusActive}
	for i := 0; i < 4; i++ {
		u.RegisterFailedLogin(now, policy)
	}
	if u.RegisterFailedLogin(now.Add(16*time.Minute), policy) {
		t.Fatal("old failures must not count")
	}
	if u.FailedLoginCount != 1 {
		t.Fatalf("count = %d", u.FailedLoginCount)
	}
}

func TestCheckCanLoginStatuses(t *testing.T) {
	now := time.Now()
	cases := map[UserStatus]string{StatusActive: "", StatusLocked: kernel.CodeAccountLocked, StatusDisabled: kernel.CodeAccountDisabled}
	for st, code := range cases {
		u := &User{Status: st, PasswordHash: "h"}
		if got := kernel.ErrorCode(u.CheckCanLogin(now)); got != code {
			t.Errorf("%s: got %q want %q", st, got, code)
		}
	}
}

func TestSessionExpiry(t *testing.T) {
	p := SessionPolicy{Idle: 12 * time.Hour, Absolute: 7 * 24 * time.Hour}
	now := time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)
	s := NewSession(kernel.NewID(), []byte{1}, "c", "", "", now, p)
	if s.Validate(now.Add(11*time.Hour), p) != nil {
		t.Fatal("should be valid within idle window")
	}
	if !errors.Is(s.Validate(now.Add(12*time.Hour), p), kernel.ErrSessionExpired) {
		t.Fatal("idle timeout")
	}
	s.LastSeenAt = now.Add(7*24*time.Hour - time.Minute)
	if !errors.Is(s.Validate(now.Add(7*24*time.Hour), p), kernel.ErrSessionExpired) {
		t.Fatal("absolute timeout")
	}
	r := now
	s2 := NewSession(kernel.NewID(), []byte{1}, "c", "", "", now, p)
	s2.RevokedAt = &r
	if s2.Validate(now, p) == nil {
		t.Fatal("revoked")
	}
}

func TestNewUserValidation(t *testing.T) {
	if _, err := NewUser("bad", "", "", []string{"ghost"}, time.Now()); err == nil {
		t.Fatal("expected validation error")
	}
	u, err := NewUser("  An@Example.COM ", "An", "h", []string{kernel.RoleReviewer}, time.Now())
	if err != nil || u.Email != "an@example.com" {
		t.Fatalf("%v %+v", err, u)
	}
	if ValidatePassword("password", "short") == nil {
		t.Fatal("short password accepted")
	}
}
