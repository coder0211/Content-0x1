// Package testutil holds fakes for shared ports, used by use-case unit
// tests of every module (no Docker needed).
package testutil

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Clock is a settable ports.Clock.
type Clock struct {
	mu sync.Mutex
	T  time.Time
}

// NewClock returns a clock fixed at 2026-10-19 09:00 UTC.
func NewClock() *Clock { return &Clock{T: time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)} }

// Now returns the current fake time.
func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.T
}

// Advance moves the clock forward.
func (c *Clock) Advance(d time.Duration) {
	c.mu.Lock()
	c.T = c.T.Add(d)
	c.mu.Unlock()
}

// Tx is a ports.TxManager that runs fn inline (no rollback). Calls counts
// invocations so tests can assert a use case opened a transaction.
type Tx struct{ Calls int }

// WithinTx runs fn.
func (t *Tx) WithinTx(ctx context.Context, fn func(context.Context) error) error {
	t.Calls++
	return fn(ctx)
}

// AuditEntry is one recorded audit call.
type AuditEntry struct {
	ActorID      kernel.ID
	Action       string
	ResourceType string
	ResourceID   string
	Before       json.RawMessage
	After        json.RawMessage
}

// Audit records audit entries in memory.
type Audit struct {
	mu      sync.Mutex
	Entries []AuditEntry
}

// Record implements ports.AuditRecorder.
func (a *Audit) Record(ctx context.Context, action, rt, rid string, before, after any) error {
	b, _ := json.Marshal(before)
	af, _ := json.Marshal(after)
	a.mu.Lock()
	defer a.mu.Unlock()
	a.Entries = append(a.Entries, AuditEntry{ActorID: kernel.ActorFrom(ctx).UserID, Action: action, ResourceType: rt, ResourceID: rid, Before: b, After: af})
	return nil
}

// Actions lists recorded action names.
func (a *Audit) Actions() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]string, len(a.Entries))
	for i, e := range a.Entries {
		out[i] = e.Action
	}
	return out
}

// Notification is one recorded notification.
type Notification struct {
	UserIDs []kernel.ID
	Type    string
	Payload any
}

// Notifier records notifications in memory.
type Notifier struct {
	mu   sync.Mutex
	Sent []Notification
}

// Notify implements ports.Notifier.
func (n *Notifier) Notify(_ context.Context, ids []kernel.ID, typ string, payload any) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.Sent = append(n.Sent, Notification{UserIDs: ids, Type: typ, Payload: payload})
	return nil
}

// Events records published job events.
type Events struct {
	mu        sync.Mutex
	Published []ports.JobEvent
}

// Publish implements ports.EventPublisher.
func (e *Events) Publish(_ context.Context, ev ports.JobEvent) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.Published = append(e.Published, ev)
	return nil
}

// Actor builds an authenticated actor with the permissions of roles.
func Actor(roles ...string) kernel.Actor {
	seen := map[string]bool{}
	var perms []string
	for _, r := range roles {
		for _, p := range kernel.RolePermissions[r] {
			if !seen[p] {
				seen[p] = true
				perms = append(perms, p)
			}
		}
	}
	return kernel.Actor{UserID: kernel.NewID(), Roles: roles, Permissions: perms, Email: "user@acs.local", Name: "Test"}
}

var (
	_ ports.Clock          = (*Clock)(nil)
	_ ports.TxManager      = (*Tx)(nil)
	_ ports.AuditRecorder  = (*Audit)(nil)
	_ ports.Notifier       = (*Notifier)(nil)
	_ ports.EventPublisher = (*Events)(nil)
)
