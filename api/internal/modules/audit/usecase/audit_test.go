package usecase_test

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"testing"

	"github.com/be/acs/api/internal/modules/audit/usecase"
	"github.com/be/acs/api/internal/shared/kernel"
	"github.com/be/acs/api/internal/testutil"
)

type memRepo struct {
	logs  []usecase.LogView
	notes map[kernel.ID][]usecase.Notification
}

func (m *memRepo) InsertLog(_ context.Context, e usecase.Entry) error {
	m.logs = append(m.logs, usecase.LogView{ID: int64(len(m.logs) + 1), ActorID: e.ActorID, Action: e.Action,
		ResourceType: e.ResourceType, ResourceID: e.ResourceID, Before: e.Before, After: e.After, IP: e.IP, RequestID: e.RequestID})
	return nil
}
func (m *memRepo) ListLogs(_ context.Context, f usecase.LogFilter, before *int64, limit int) ([]usecase.LogView, error) {
	var out []usecase.LogView
	for _, l := range m.logs {
		if (before == nil || l.ID < *before) && (f.Action == "" || l.Action == f.Action) {
			out = append(out, l)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
func (m *memRepo) InsertNotifications(_ context.Context, ids []kernel.ID, typ string, p json.RawMessage) error {
	for _, id := range ids {
		m.notes[id] = append(m.notes[id], usecase.Notification{ID: kernel.NewID(), Type: typ, Payload: p})
	}
	return nil
}
func (m *memRepo) ListNotifications(_ context.Context, uid kernel.ID, _ bool, p kernel.PageRequest) (kernel.Page[usecase.Notification], error) {
	return kernel.PageFromRows(m.notes[uid], p.Limit, func(n usecase.Notification) any { return n.ID }), nil
}
func (m *memRepo) CountUnread(_ context.Context, uid kernel.ID) (int64, error) {
	return int64(len(m.notes[uid])), nil
}
func (m *memRepo) MarkRead(_ context.Context, id, uid kernel.ID) error {
	for _, n := range m.notes[uid] {
		if n.ID == id {
			return nil
		}
	}
	return kernel.ErrNotFound
}

func TestRecordTakesActorAndRequestFromContext(t *testing.T) {
	repo := &memRepo{notes: map[kernel.ID][]usecase.Notification{}}
	svc := &usecase.Service{Repo: repo}
	actor := testutil.Actor(kernel.RoleAdmin)
	ctx := kernel.WithActor(context.Background(), actor)
	ctx = kernel.WithRequestMeta(ctx, kernel.RequestMeta{RequestID: "r1", IP: "10.0.0.1"})
	if err := svc.Record(ctx, "budget.update", "budget", "global", map[string]int{"limit": 1}, map[string]int{"limit": 2}); err != nil {
		t.Fatal(err)
	}
	l := repo.logs[0]
	if *l.ActorID != actor.UserID || l.RequestID != "r1" || l.IP != "10.0.0.1" || string(l.After) != `{"limit":2}` {
		t.Fatalf("%+v", l)
	}
}

func TestListLogsRequiresAuditView(t *testing.T) {
	repo := &memRepo{notes: map[kernel.ID][]usecase.Notification{}}
	svc := &usecase.Service{Repo: repo}
	for i := 0; i < 3; i++ {
		_ = svc.Record(context.Background(), "asset.download", "asset", "x", nil, nil)
	}
	if _, err := svc.ListLogs(context.Background(), testutil.Actor(kernel.RoleMarketingLead), usecase.LogFilter{}, kernel.PageRequest{}); !errors.Is(err, kernel.ErrForbidden) {
		t.Fatalf("lead: %v", err)
	}
	p, err := svc.ListLogs(context.Background(), testutil.Actor(kernel.RoleAdmin), usecase.LogFilter{}, kernel.PageRequest{Limit: 2})
	if err != nil || len(p.Items) != 2 || p.NextCursor == nil || p.Items[0].ID != 3 {
		t.Fatalf("%v %+v", err, p)
	}
	p2, _ := svc.ListLogs(context.Background(), testutil.Actor(kernel.RoleAdmin), usecase.LogFilter{}, kernel.PageRequest{Limit: 2, Cursor: *p.NextCursor})
	if len(p2.Items) != 1 || p2.Items[0].ID != 1 || p2.NextCursor != nil {
		t.Fatalf("%+v", p2)
	}
}

func TestNotifications(t *testing.T) {
	repo := &memRepo{notes: map[kernel.ID][]usecase.Notification{}}
	svc := &usecase.Service{Repo: repo}
	u := testutil.Actor(kernel.RoleReviewer)
	if err := svc.Notify(context.Background(), []kernel.ID{u.UserID}, "review_requested", map[string]string{"asset_id": "a"}); err != nil {
		t.Fatal(err)
	}
	p, unread, err := svc.ListNotifications(context.Background(), u, false, kernel.PageRequest{})
	if err != nil || len(p.Items) != 1 || unread != 1 {
		t.Fatalf("%v %+v %d", err, p, unread)
	}
	other := testutil.Actor(kernel.RoleReviewer)
	if err := svc.MarkRead(context.Background(), other, p.Items[0].ID); !errors.Is(err, kernel.ErrNotFound) {
		t.Fatal("IDOR: marking someone else's notification must be NOT_FOUND")
	}
}
