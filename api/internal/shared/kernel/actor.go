package kernel

import (
	"context"
	"slices"
)

// Actor is the authenticated user on whose behalf a use case runs.
type Actor struct {
	UserID      ID
	Email       string
	Name        string
	Roles       []string
	Permissions []string
	SessionID   ID
}

// System is the actor used by workers, CLIs and maintenance tasks.
var System = Actor{Roles: []string{"system"}}

// IsAuthenticated reports whether the actor is a real, logged-in user.
func (a Actor) IsAuthenticated() bool { return a.UserID != NilID }

// Can reports whether the actor holds permission perm.
func (a Actor) Can(perm string) bool { return slices.Contains(a.Permissions, perm) }

// HasRole reports whether the actor holds role code.
func (a Actor) HasRole(code string) bool { return slices.Contains(a.Roles, code) }

// IsAdmin is shorthand for HasRole(RoleAdmin).
func (a Actor) IsAdmin() bool { return a.HasRole(RoleAdmin) }

// IsLeadOrAdmin reports whether the actor may act on everyone's resources
// ("Lead/Admin sửa tất cả", SPEC-01 §2).
func (a Actor) IsLeadOrAdmin() bool { return a.IsAdmin() || a.HasRole(RoleMarketingLead) }

// Require returns ErrForbidden unless the actor holds perm.
func (a Actor) Require(perm string) error {
	if !a.IsAuthenticated() {
		return ErrUnauthenticated
	}
	if !a.Can(perm) {
		return ErrForbidden
	}
	return nil
}

// CanModifyOwned implements the ownership rule shared by assets, jobs and
// files: owners may modify their own resources, Lead/Admin may modify all.
func (a Actor) CanModifyOwned(ownerID ID) bool {
	return a.IsAuthenticated() && (a.UserID == ownerID || a.IsLeadOrAdmin())
}

// RequestMeta carries per-request metadata used by logging and audit.
type RequestMeta struct {
	RequestID string
	IP        string
	UserAgent string
}

type actorKey struct{}
type requestMetaKey struct{}

// WithActor stores the actor in ctx (set by the auth middleware).
func WithActor(ctx context.Context, a Actor) context.Context {
	return context.WithValue(ctx, actorKey{}, a)
}

// ActorFrom returns the actor stored in ctx, or the zero Actor.
func ActorFrom(ctx context.Context) Actor {
	a, _ := ctx.Value(actorKey{}).(Actor)
	return a
}

// WithRequestMeta stores request metadata in ctx.
func WithRequestMeta(ctx context.Context, m RequestMeta) context.Context {
	return context.WithValue(ctx, requestMetaKey{}, m)
}

// RequestMetaFrom returns request metadata stored in ctx.
func RequestMetaFrom(ctx context.Context) RequestMeta {
	m, _ := ctx.Value(requestMetaKey{}).(RequestMeta)
	return m
}
