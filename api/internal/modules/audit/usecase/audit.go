package usecase

import (
	"context"
	"encoding/json"
	"time"

	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Entry is one audit_logs row to append.
type Entry struct {
	ActorID      *kernel.ID
	Action       string
	ResourceType string
	ResourceID   string
	Before       json.RawMessage
	After        json.RawMessage
	IP           string
	RequestID    string
}

// LogView is an audit row for GET /audit-logs.
type LogView struct {
	ID           int64
	ActorID      *kernel.ID
	ActorEmail   string
	Action       string
	ResourceType string
	ResourceID   string
	Before       json.RawMessage
	After        json.RawMessage
	IP           string
	RequestID    string
	CreatedAt    time.Time
}

// LogFilter filters audit logs.
type LogFilter struct {
	ActorID      *kernel.ID
	ResourceType string
	ResourceID   string
	Action       string
	From         *time.Time
	To           *time.Time
}

// Notification is an in-app notification.
type Notification struct {
	ID        kernel.ID
	Type      string
	Payload   json.RawMessage
	ReadAt    *time.Time
	CreatedAt time.Time
}

// Repository persists audit logs and notifications.
type Repository interface {
	InsertLog(ctx context.Context, e Entry) error
	ListLogs(ctx context.Context, f LogFilter, beforeID *int64, limit int) ([]LogView, error)
	InsertNotifications(ctx context.Context, userIDs []kernel.ID, typ string, payload json.RawMessage) error
	ListNotifications(ctx context.Context, userID kernel.ID, unreadOnly bool, page kernel.PageRequest) (kernel.Page[Notification], error)
	CountUnread(ctx context.Context, userID kernel.ID) (int64, error)
	// MarkRead returns NOT_FOUND unless the notification belongs to userID.
	MarkRead(ctx context.Context, id, userID kernel.ID) error
}

// Service implements API plus the audit/notification use cases.
type Service struct {
	Repo Repository
}

var _ API = (*Service)(nil)

func marshal(v any) (json.RawMessage, error) {
	if v == nil {
		return nil, nil
	}
	if raw, ok := v.(json.RawMessage); ok {
		return raw, nil
	}
	return json.Marshal(v)
}

// Record implements ports.AuditRecorder; actor and request metadata come
// from ctx. It joins the caller's transaction via ctx.
func (s *Service) Record(ctx context.Context, action, resourceType, resourceID string, before, after any) error {
	b, err := marshal(before)
	if err != nil {
		return err
	}
	a, err := marshal(after)
	if err != nil {
		return err
	}
	meta := kernel.RequestMetaFrom(ctx)
	return s.Repo.InsertLog(ctx, Entry{
		ActorID: kernel.IDPtr(kernel.ActorFrom(ctx).UserID), Action: action, ResourceType: resourceType,
		ResourceID: resourceID, Before: b, After: a, IP: meta.IP, RequestID: meta.RequestID,
	})
}

// Notify implements ports.Notifier.
func (s *Service) Notify(ctx context.Context, userIDs []kernel.ID, typ string, payload any) error {
	if len(userIDs) == 0 {
		return nil
	}
	p, err := marshal(payload)
	if err != nil {
		return err
	}
	if p == nil {
		p = json.RawMessage(`{}`)
	}
	return s.Repo.InsertNotifications(ctx, userIDs, typ, p)
}

type logCursor struct {
	ID int64 `json:"i"`
}

// ListLogs pages through audit logs (audit.view).
func (s *Service) ListLogs(ctx context.Context, actor kernel.Actor, f LogFilter, page kernel.PageRequest) (kernel.Page[LogView], error) {
	if err := actor.Require(kernel.PermAuditView); err != nil {
		return kernel.Page[LogView]{}, err
	}
	page = page.Normalized()
	var c logCursor
	has, err := kernel.DecodeCursor(page.Cursor, &c)
	if err != nil {
		return kernel.Page[LogView]{}, err
	}
	var before *int64
	if has {
		before = &c.ID
	}
	rows, err := s.Repo.ListLogs(ctx, f, before, page.Limit+1)
	if err != nil {
		return kernel.Page[LogView]{}, err
	}
	return kernel.PageFromRows(rows, page.Limit, func(l LogView) any { return logCursor{ID: l.ID} }), nil
}

// ListNotifications pages the actor's notifications.
func (s *Service) ListNotifications(ctx context.Context, actor kernel.Actor, unreadOnly bool, page kernel.PageRequest) (kernel.Page[Notification], int64, error) {
	if !actor.IsAuthenticated() {
		return kernel.Page[Notification]{}, 0, kernel.ErrUnauthenticated
	}
	p, err := s.Repo.ListNotifications(ctx, actor.UserID, unreadOnly, page.Normalized())
	if err != nil {
		return p, 0, err
	}
	n, err := s.Repo.CountUnread(ctx, actor.UserID)
	return p, n, err
}

// MarkRead marks one of the actor's notifications as read.
func (s *Service) MarkRead(ctx context.Context, actor kernel.Actor, id kernel.ID) error {
	if !actor.IsAuthenticated() {
		return kernel.ErrUnauthenticated
	}
	return s.Repo.MarkRead(ctx, id, actor.UserID)
}

var _ ports.Notifier = (*Service)(nil)
