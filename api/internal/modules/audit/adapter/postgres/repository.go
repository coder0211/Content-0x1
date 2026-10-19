package postgres

import (
	"context"
	"encoding/json"
	"net/netip"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/be/acs/api/internal/modules/audit/usecase"
	db "github.com/be/acs/api/internal/platform/postgres"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Repository implements usecase.Repository.
type Repository struct{ pool *pgxpool.Pool }

// NewRepository returns a repository on pool.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

var _ usecase.Repository = (*Repository)(nil)

func (r *Repository) q(ctx context.Context) *Queries { return New(db.Conn(ctx, r.pool)) }

func optStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// InsertLog appends an audit row.
func (r *Repository) InsertLog(ctx context.Context, e usecase.Entry) error {
	var ip *netip.Addr
	if a, err := netip.ParseAddr(e.IP); err == nil {
		ip = &a
	}
	return r.q(ctx).InsertAuditLog(ctx, InsertAuditLogParams{
		ActorID: e.ActorID, Action: e.Action, ResourceType: e.ResourceType, ResourceID: e.ResourceID,
		BeforeJson: e.Before, AfterJson: e.After, Ip: ip, RequestID: optStr(e.RequestID),
	})
}

// ListLogs returns logs newest first.
func (r *Repository) ListLogs(ctx context.Context, f usecase.LogFilter, beforeID *int64, limit int) ([]usecase.LogView, error) {
	rows, err := r.q(ctx).ListAuditLogs(ctx, ListAuditLogsParams{
		ActorID: f.ActorID, ResourceType: optStr(f.ResourceType), ResourceID: optStr(f.ResourceID), Action: optStr(f.Action),
		FromAt: f.From, ToAt: f.To, BeforeID: beforeID, Lim: int32(limit),
	})
	if err != nil {
		return nil, err
	}
	out := make([]usecase.LogView, 0, len(rows))
	for _, row := range rows {
		v := usecase.LogView{ID: row.ID, ActorID: row.ActorID, Action: row.Action, ResourceType: row.ResourceType,
			ResourceID: row.ResourceID, Before: row.BeforeJson, After: row.AfterJson, IP: row.Ip, CreatedAt: row.CreatedAt}
		if row.ActorEmail != nil {
			v.ActorEmail = *row.ActorEmail
		}
		if row.RequestID != nil {
			v.RequestID = *row.RequestID
		}
		out = append(out, v)
	}
	return out, nil
}

// InsertNotifications fans one notification out to several users.
func (r *Repository) InsertNotifications(ctx context.Context, ids []kernel.ID, typ string, payload json.RawMessage) error {
	return r.q(ctx).InsertNotifications(ctx, InsertNotificationsParams{UserIds: ids, Type: typ, Payload: payload})
}

// ListNotifications pages a user's notifications.
func (r *Repository) ListNotifications(ctx context.Context, userID kernel.ID, unreadOnly bool, page kernel.PageRequest) (kernel.Page[usecase.Notification], error) {
	var c kernel.TimeIDCursor
	has, err := kernel.DecodeCursor(page.Cursor, &c)
	if err != nil {
		return kernel.Page[usecase.Notification]{}, err
	}
	p := ListNotificationsParams{UserID: userID, UnreadOnly: unreadOnly, Lim: int32(page.Limit + 1)}
	if has {
		p.CursorAt, p.CursorID = &c.At, &c.ID
	}
	rows, err := r.q(ctx).ListNotifications(ctx, p)
	if err != nil {
		return kernel.Page[usecase.Notification]{}, err
	}
	items := make([]usecase.Notification, 0, len(rows))
	for _, row := range rows {
		items = append(items, usecase.Notification{ID: row.ID, Type: row.Type, Payload: row.Payload, ReadAt: row.ReadAt, CreatedAt: row.CreatedAt})
	}
	return kernel.PageFromRows(items, page.Limit, func(n usecase.Notification) any {
		return kernel.TimeIDCursor{At: n.CreatedAt, ID: n.ID}
	}), nil
}

// CountUnread counts unread notifications.
func (r *Repository) CountUnread(ctx context.Context, userID kernel.ID) (int64, error) {
	return r.q(ctx).CountUnreadNotifications(ctx, userID)
}

// MarkRead marks a notification read if it belongs to userID.
func (r *Repository) MarkRead(ctx context.Context, id, userID kernel.ID) error {
	n, err := r.q(ctx).MarkNotificationRead(ctx, MarkNotificationReadParams{ID: id, UserID: userID})
	if err != nil {
		return err
	}
	if n == 0 {
		return kernel.ErrNotFound
	}
	return nil
}
