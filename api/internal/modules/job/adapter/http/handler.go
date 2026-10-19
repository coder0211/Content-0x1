// Package http is the job HTTP adapter: GET /jobs, GET /jobs/{id},
// GET /jobs/{id}/events (SSE), GET /jobs/{id}/history, POST cancel/retry.
package http

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/be/acs/api/internal/modules/job/usecase"
	"github.com/be/acs/api/internal/platform/httpx"
	"github.com/be/acs/api/internal/platform/sse"
	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Handler serves the job endpoints.
type Handler struct {
	Svc        *usecase.Service
	Subscriber ports.EventSubscriber
	// UserNames resolves creator display names (nil: names stay empty).
	UserNames func(ctx context.Context, ids []kernel.ID) map[kernel.ID]string
}

func (h *Handler) names(ctx context.Context, ids []kernel.ID) map[kernel.ID]string {
	if h.UserNames == nil || len(ids) == 0 {
		return map[kernel.ID]string{}
	}
	return h.UserNames(ctx, ids)
}

// Routes registers routes on the /api/v1 router. Authorization (owner or
// provider.view; owner or admin for cancel/retry) is enforced by the use
// case so other users' jobs are 404.
func (h *Handler) Routes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(httpx.RequireAuth)
		r.Get("/jobs", h.list)
		r.Get("/jobs/{id}", h.get)
		r.Get("/jobs/{id}/events", h.events)
		r.Get("/jobs/{id}/history", h.history)
		r.Post("/jobs/{id}/cancel", h.cancel)
		r.Post("/jobs/{id}/retry", h.retry)
	})
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

var validStatuses = map[usecase.Status]bool{
	usecase.StatusQueued: true, usecase.StatusRunning: true, usecase.StatusSucceeded: true,
	usecase.StatusFailed: true, usecase.StatusCancelled: true,
}

func parseFilter(r *http.Request) (usecase.JobFilter, error) {
	q := r.URL.Query()
	var f usecase.JobFilter
	var v kernel.ValidationErrors
	for _, s := range splitList(q.Get("status")) {
		if !validStatuses[usecase.Status(s)] {
			v.Add("status", "invalid")
			continue
		}
		f.Status = append(f.Status, usecase.Status(s))
	}
	for _, t := range splitList(q.Get("type")) {
		if !usecase.IsKnownJobType(usecase.JobType(t)) {
			v.Add("type", "invalid")
			continue
		}
		f.Type = append(f.Type, usecase.JobType(t))
	}
	if err := v.Err(); err != nil {
		return f, err
	}
	var err error
	if f.CampaignID, err = httpx.QueryID(r, "campaign_id"); err != nil {
		return f, err
	}
	if f.CreatedBy, err = httpx.QueryID(r, "created_by"); err != nil {
		return f, err
	}
	f.ProviderCode = strings.TrimSpace(q.Get("provider_code"))
	switch q.Get("needs_reconciliation") {
	case "":
	case "true":
		b := true
		f.NeedsReconciliation = &b
	case "false":
		b := false
		f.NeedsReconciliation = &b
	default:
		return f, kernel.Validation(kernel.FieldIssue("needs_reconciliation", "invalid"))
	}
	return f, nil
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	f, err := parseFilter(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	page, err := httpx.PageRequest(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	res, err := h.Svc.List(r.Context(), httpx.ActorFrom(r), f, page)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	items := make([]jobSummaryDTO, 0, len(res.Items))
	ids := make([]kernel.ID, 0, len(res.Items))
	for _, j := range res.Items {
		ids = append(ids, j.CreatedBy)
	}
	names := h.names(r.Context(), ids)
	for _, j := range res.Items {
		d := summaryDTO(j.JobView, j.NeedsReconciliation)
		d.CreatedByName = names[j.CreatedBy]
		items = append(items, d)
	}
	httpx.List(w, r, items, res.NextCursor)
}

// detail hides internal step error details from actors without
// provider.view (owners see error_kind and the job's Vietnamese message).
func detail(actor kernel.Actor, d usecase.JobDetail) jobDTO {
	out := detailDTO(d)
	if !actor.Can(kernel.PermProviderView) {
		for i := range out.Steps {
			out.Steps[i].ErrorMessage = nil
		}
	}
	return out
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	actor := httpx.ActorFrom(r)
	d, err := h.Svc.GetForActor(r.Context(), actor, id)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := detail(actor, d)
	out.CreatedByName = h.names(r.Context(), []kernel.ID{d.CreatedBy})[d.CreatedBy]
	httpx.OK(w, r, out)
}

func (h *Handler) history(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	limit, err := httpx.QueryInt(r, "limit", 200)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	evs, err := h.Svc.History(r.Context(), httpx.ActorFrom(r), id, limit)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	items := make([]eventDTO, 0, len(evs))
	for _, e := range evs {
		items = append(items, eventDTO{ID: e.ID, StepID: e.StepID, EventType: e.Type, Payload: rawOrNull(e.Payload), CreatedAt: e.CreatedAt})
	}
	httpx.List(w, r, items, nil)
}

func (h *Handler) cancel(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := h.Svc.RequestCancel(r.Context(), httpx.ActorFrom(r), id); err != nil {
		httpx.Error(w, r, err)
		return
	}
	v, err := h.Svc.Get(r.Context(), id)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.Accepted(w, r, cancelAcceptedDTO{JobID: v.ID, Status: string(v.Status), CancelRequested: true})
}

func (h *Handler) retry(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var req retryRequest
	if r.ContentLength != 0 {
		if err := httpx.Decode(r, &req); err != nil {
			httpx.Error(w, r, err)
			return
		}
	}
	res, err := h.Svc.Retry(r.Context(), httpx.ActorFrom(r), id, req.Force)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := jobAcceptedDTO{JobID: res.JobID, Status: string(res.Status)}
	if res.CostEstimate.Currency != "" {
		out.EstimatedCost = &moneyDTO{Amount: json.Number(res.CostEstimate.Amount.String()), Currency: res.CostEstimate.Currency, IsEstimate: true}
	}
	httpx.Accepted(w, r, out)
}

func isDone(ev sse.Event) bool { return ev.Name == ports.JobEventDone }

func snapshotEvents(d usecase.JobDetail) []sse.Event {
	st, _ := json.Marshal(map[string]any{"job_id": d.ID, "status": d.Status, "progress": d.Progress, "cancel_requested": d.CancelRequested})
	out := []sse.Event{{Name: ports.JobEventStatus, Data: st}}
	if d.Status.IsTerminal() {
		done := map[string]any{"job_id": d.ID, "status": d.Status, "result": rawOrNull(d.Result)}
		if d.ErrorCode != "" {
			done["error"] = errorDTO{Code: d.ErrorCode, Message: d.ErrorMessage}
		}
		b, _ := json.Marshal(done)
		out = append(out, sse.Event{Name: ports.JobEventDone, Data: b})
	}
	return out
}

// events streams job events (SPEC-04 §3). It subscribes before reading the
// snapshot so no transition is lost, sends the current status first, and
// ends after the terminal "done" event. Without Redis it sends only the
// snapshot; the FE then falls back to polling.
func (h *Handler) events(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	actor := httpx.ActorFrom(r)
	ctx := r.Context()
	if _, err := h.Svc.GetForActor(ctx, actor, id); err != nil {
		httpx.Error(w, r, err)
		return
	}
	var sub <-chan ports.JobEvent
	if h.Subscriber != nil {
		ch, closeSub, err := h.Subscriber.Subscribe(ctx, id)
		if err != nil {
			httpx.Error(w, r, kernel.ErrUnavailable)
			return
		}
		defer closeSub()
		sub = ch
	}
	d, err := h.Svc.GetForActor(ctx, actor, id)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	sw, err := sse.Start(w)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	for _, ev := range snapshotEvents(d) {
		if err := sw.Send(ev); err != nil || isDone(ev) {
			return
		}
	}
	if sub == nil {
		return
	}
	out := make(chan sse.Event)
	go forward(ctx, sub, out)
	_ = sw.Pump(ctx, out, isDone)
}

func forward(ctx context.Context, in <-chan ports.JobEvent, out chan<- sse.Event) {
	defer close(out)
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-in:
			if !ok {
				return
			}
			select {
			case out <- sse.Event{Name: ev.Type, Data: ev.Data}:
			case <-ctx.Done():
				return
			}
		}
	}
}
