package budgettest

import (
	"context"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/shopspring/decimal"

	"github.com/be/acs/api/internal/modules/budget/domain"
	"github.com/be/acs/api/internal/modules/budget/usecase"
	"github.com/be/acs/api/internal/shared/kernel"
)

// PriceRow is one in-memory model_pricing row.
type PriceRow struct {
	Provider, Model, Unit string
	Price                 decimal.Decimal
	Version               string
	From                  time.Time
	To                    *time.Time
}

// JobCost mirrors the ai_jobs cost columns written by Settle/RecordUsage.
type JobCost struct {
	Actual              decimal.Decimal
	Source              usecase.CostSource
	NeedsReconciliation bool
}

type memUsage struct {
	usecase.UsageRow
	settled bool
}

type memEmail struct {
	usecase.AlertEmail
	sent bool
}

// Store is an in-memory usecase.Repository that is also a ports.TxManager
// with row-level locks: GetXxx(forUpdate=true) takes the budget row's mutex
// and holds it until the outermost WithinTx returns, like SELECT … FOR UPDATE.
// There is no rollback: use cases only write after their checks pass.
type Store struct {
	mu        sync.Mutex
	rowLocks  map[kernel.ID]*sync.Mutex
	Settings  domain.Settings
	Budgets   map[kernel.ID]domain.Budget
	Prices    []PriceRow
	Defaults  map[string][2]string // kind → provider, model
	Campaigns map[kernel.ID]bool   // existing campaigns (FK); nil = any
	// CampaignLabels are campaign names for the usage summary.
	CampaignLabels map[kernel.ID]string
	resv           map[kernel.ID]*usecase.ReservationRow
	usage          []memUsage
	Jobs           map[kernel.ID]JobCost
	emails         []*memEmail
}

// NewStore returns a store seeded like migration 0011 (OQ-03 settings, fake pricing).
func NewStore() *Store {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	p := func(model, unit, price string) PriceRow {
		return PriceRow{Provider: "fake", Model: model, Unit: unit, Price: decimal.RequireFromString(price), Version: "fake-2026-10", From: from}
	}
	return &Store{
		rowLocks: map[kernel.ID]*sync.Mutex{},
		Settings: domain.Settings{PerJobText: decimal.RequireFromString("0.50"), PerJobImage: decimal.NewFromInt(2), PerJobVideo: decimal.NewFromInt(8),
			GlobalMonthlyLimit: decimal.NewFromInt(1000), GlobalThresholds: []int{80, 100}, GlobalHardStop: true, MaxConcurrentVideoJobs: 3, Currency: "USD"},
		Budgets: map[kernel.ID]domain.Budget{},
		Prices: []PriceRow{
			p("fake-text", "input_token", "0.000003"), p("fake-text", "output_token", "0.000015"),
			p("fake-image", "image_1024_standard", "0.04"), p("fake-image", "image_1024_high", "0.17"),
			p("fake-video", "video_second_720p", "0.10"), p("fake-video", "video_second_1080p", "0.15"),
			p("fake-voice", "char", "0.000016"),
		},
		Defaults: map[string][2]string{"text": {"fake", "fake-text"}, "image": {"fake", "fake-image"}, "video": {"fake", "fake-video"}, "voice": {"fake", "fake-voice"}},
		resv:     map[kernel.ID]*usecase.ReservationRow{},
		Jobs:     map[kernel.ID]JobCost{},
	}
}

var _ usecase.Repository = (*Store)(nil)

// ---- TxManager with row locks

type txKey struct{}

type txState struct{ held []*sync.Mutex }

// WithinTx runs fn; nested calls join the outer transaction. Row locks taken
// inside are released when the outermost call returns.
func (s *Store) WithinTx(ctx context.Context, fn func(context.Context) error) error {
	if _, ok := ctx.Value(txKey{}).(*txState); ok {
		return fn(ctx)
	}
	st := &txState{}
	defer func() {
		for i := len(st.held) - 1; i >= 0; i-- {
			st.held[i].Unlock()
		}
	}()
	return fn(context.WithValue(ctx, txKey{}, st))
}

func (s *Store) lockRow(ctx context.Context, id kernel.ID) {
	st, ok := ctx.Value(txKey{}).(*txState)
	if !ok {
		panic("budgettest.Store: FOR UPDATE outside a transaction")
	}
	s.mu.Lock()
	m, ok := s.rowLocks[id]
	if !ok {
		m = &sync.Mutex{}
		s.rowLocks[id] = m
	}
	s.mu.Unlock()
	if slices.Contains(st.held, m) {
		return
	}
	m.Lock()
	st.held = append(st.held, m)
}

// ---- pricing

func (s *Store) EffectivePrice(_ context.Context, provider, model, unit string, at time.Time) (usecase.Price, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var best *PriceRow
	for i := range s.Prices {
		r := &s.Prices[i]
		if r.Provider == provider && r.Model == model && r.Unit == unit && !r.From.After(at) && (r.To == nil || r.To.After(at)) {
			if best == nil || r.From.After(best.From) {
				best = r
			}
		}
	}
	if best == nil {
		return usecase.Price{}, kernel.ErrNotFound
	}
	return usecase.Price{UnitPrice: best.Price, Currency: "USD", Version: best.Version}, nil
}

func (s *Store) DefaultModel(_ context.Context, kind string) (string, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.Defaults[kind]
	if !ok {
		return "", "", kernel.ErrNotFound
	}
	return d[0], d[1], nil
}

// ---- settings

func (s *Store) GetSettings(context.Context) (domain.Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.Settings
	st.GlobalThresholds = slices.Clone(st.GlobalThresholds)
	return st, nil
}

func (s *Store) SaveSettings(_ context.Context, st domain.Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Settings = st
	return nil
}

// ---- budgets

func clone(b domain.Budget) domain.Budget {
	b.Thresholds, b.AlertsSent = slices.Clone(b.Thresholds), slices.Clone(b.AlertsSent)
	return b
}

func (s *Store) EnsureGlobalBudget(_ context.Context, b domain.Budget) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, x := range s.Budgets {
		if x.Scope == domain.ScopeGlobal && x.PeriodStart.Equal(b.PeriodStart) {
			return nil
		}
	}
	s.Budgets[b.ID] = clone(b)
	return nil
}

func (s *Store) find(match func(domain.Budget) bool) (domain.Budget, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, x := range s.Budgets {
		if match(x) {
			return clone(x), true
		}
	}
	return domain.Budget{}, false
}

func (s *Store) getLocked(ctx context.Context, forUpdate bool, match func(domain.Budget) bool) (domain.Budget, error) {
	b, ok := s.find(match)
	if !ok {
		return domain.Budget{}, kernel.ErrNotFound
	}
	if forUpdate {
		s.lockRow(ctx, b.ID)
		b, _ = s.find(func(x domain.Budget) bool { return x.ID == b.ID }) // re-read after the lock
	}
	return b, nil
}

func (s *Store) GetGlobalBudget(ctx context.Context, start time.Time, forUpdate bool) (domain.Budget, error) {
	return s.getLocked(ctx, forUpdate, func(x domain.Budget) bool { return x.Scope == domain.ScopeGlobal && x.PeriodStart.Equal(start) })
}

func (s *Store) GetCampaignBudget(ctx context.Context, id kernel.ID, forUpdate bool) (domain.Budget, error) {
	return s.getLocked(ctx, forUpdate, func(x domain.Budget) bool { return x.CampaignID != nil && *x.CampaignID == id })
}

func (s *Store) GetBudget(ctx context.Context, id kernel.ID, forUpdate bool) (domain.Budget, error) {
	return s.getLocked(ctx, forUpdate, func(x domain.Budget) bool { return x.ID == id })
}

func (s *Store) ListCampaignBudgets(context.Context) ([]domain.Budget, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []domain.Budget{}
	for _, x := range s.Budgets {
		if x.Scope == domain.ScopeCampaign {
			out = append(out, clone(x))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}

func (s *Store) InsertBudget(_ context.Context, b domain.Budget) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if b.CampaignID != nil && s.Campaigns != nil && !s.Campaigns[*b.CampaignID] {
		return kernel.ErrNotFound
	}
	for _, x := range s.Budgets {
		if x.CampaignID != nil && b.CampaignID != nil && *x.CampaignID == *b.CampaignID {
			return kernel.ErrConflict
		}
	}
	s.Budgets[b.ID] = clone(b)
	return nil
}

func (s *Store) SaveBudget(_ context.Context, b domain.Budget) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.Budgets[b.ID]; !ok {
		return kernel.ErrNotFound
	}
	s.Budgets[b.ID] = clone(b)
	return nil
}

// CampaignNames returns CampaignLabels entries for ids.
func (s *Store) CampaignNames(_ context.Context, ids []kernel.ID) (map[kernel.ID]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[kernel.ID]string{}
	for _, id := range ids {
		if n, ok := s.CampaignLabels[id]; ok {
			out[id] = n
		}
	}
	return out, nil
}

// ---- reservations

func (s *Store) HeldReservations(_ context.Context, jobID kernel.ID) ([]usecase.ReservationRow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []usecase.ReservationRow{}
	for _, r := range s.resv {
		if r.JobID == jobID && r.Status == "held" {
			out = append(out, *r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Scope == domain.ScopeGlobal && out[j].Scope != domain.ScopeGlobal })
	return out, nil
}

func (s *Store) UpsertReservation(_ context.Context, budgetID, jobID kernel.ID, amount decimal.Decimal) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.resv {
		if r.BudgetID == budgetID && r.JobID == jobID {
			r.Amount, r.Status = amount, "held"
			return nil
		}
	}
	id := kernel.NewID()
	s.resv[id] = &usecase.ReservationRow{ID: id, BudgetID: budgetID, Scope: s.Budgets[budgetID].Scope, JobID: jobID, Amount: amount, Status: "held"}
	return nil
}

func (s *Store) SetReservationStatus(_ context.Context, id kernel.ID, status string, _ time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r, ok := s.resv[id]; ok {
		r.Status = status
	}
	return nil
}

// Reservations returns a snapshot of every reservation row.
func (s *Store) Reservations() []usecase.ReservationRow {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []usecase.ReservationRow{}
	for _, r := range s.resv {
		out = append(out, *r)
	}
	return out
}

// ---- usage

func (s *Store) InsertUsage(_ context.Context, u usecase.UsageRow) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.usage = append(s.usage, memUsage{UsageRow: u})
	return nil
}

// Usage returns a snapshot of all usage rows.
func (s *Store) Usage() []usecase.UsageRow {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]usecase.UsageRow, 0, len(s.usage))
	for _, u := range s.usage {
		out = append(out, u.UsageRow)
	}
	return out
}

func (s *Store) UnsettledUsage(_ context.Context, jobID kernel.ID) (usecase.UnsettledUsage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := usecase.UnsettledUsage{Total: decimal.Zero}
	for _, u := range s.usage {
		if u.JobID == jobID && !u.settled {
			out.Total = out.Total.Add(u.Cost)
			out.Count++
			if out.CampaignID == nil {
				out.CampaignID = u.CampaignID
			}
		}
	}
	return out, nil
}

func (s *Store) MarkUsageSettled(_ context.Context, jobID kernel.ID, _ time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.usage {
		if s.usage[i].JobID == jobID {
			s.usage[i].settled = true
		}
	}
	return nil
}

func (s *Store) JobUsageTotals(_ context.Context, jobID kernel.ID) (usecase.JobUsageTotals, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := usecase.JobUsageTotals{Total: decimal.Zero}
	for _, u := range s.usage {
		if u.JobID != jobID {
			continue
		}
		out.Total = out.Total.Add(u.Cost)
		out.Count++
		switch u.Source {
		case usecase.SourceProviderReported:
			out.ProviderReported++
		case usecase.SourceCalculated:
			out.Calculated++
		case usecase.SourceEstimated:
			out.Estimated++
		}
	}
	return out, nil
}

func (s *Store) UpdateJobCost(_ context.Context, jobID kernel.ID, actual decimal.Decimal, source usecase.CostSource, recon bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	j := s.Jobs[jobID]
	j.Actual, j.Source = actual, source
	j.NeedsReconciliation = j.NeedsReconciliation || recon
	s.Jobs[jobID] = j
	return nil
}

func (s *Store) MarkJobNeedsReconciliation(_ context.Context, jobID kernel.ID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	j := s.Jobs[jobID]
	j.NeedsReconciliation = true
	s.Jobs[jobID] = j
	return nil
}

func (s *Store) UsageSummary(_ context.Context, f usecase.SummaryFilter) ([]usecase.SummaryCell, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	type k struct {
		key string
		src usecase.CostSource
	}
	agg := map[k]*usecase.SummaryCell{}
	var order []k
	for _, u := range s.usage {
		if u.OccurredAt.Before(f.From) || !u.OccurredAt.Before(f.To) {
			continue
		}
		var key string
		switch f.GroupBy {
		case "day":
			key = u.OccurredAt.UTC().Format("2006-01-02")
		case "campaign":
			if u.CampaignID != nil {
				key = u.CampaignID.String()
			}
		case "provider":
			key = u.ProviderCode
		case "model":
			key = u.ModelCode
		case "user":
			key = u.UserID.String()
		case "job_type":
			key = u.JobType
		}
		kk := k{key, u.Source}
		c, ok := agg[kk]
		if !ok {
			c = &usecase.SummaryCell{Key: key, Source: u.Source, Amount: decimal.Zero}
			agg[kk] = c
			order = append(order, kk)
		}
		c.Amount = c.Amount.Add(u.Cost)
		c.Count++
	}
	out := make([]usecase.SummaryCell, 0, len(order))
	for _, kk := range order {
		out = append(out, *agg[kk])
	}
	return out, nil
}

// ---- alert e-mails

func (s *Store) QueueAlertEmail(_ context.Context, e usecase.AlertEmail) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.emails = append(s.emails, &memEmail{AlertEmail: e})
	return nil
}

func (s *Store) PendingAlertEmails(_ context.Context, maxAttempts, limit int) ([]usecase.AlertEmail, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []usecase.AlertEmail{}
	for _, e := range s.emails {
		if !e.sent && e.Attempts < maxAttempts && len(out) < limit {
			out = append(out, e.AlertEmail)
		}
	}
	return out, nil
}

func (s *Store) MarkAlertEmailSent(_ context.Context, id kernel.ID, _ time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.emails {
		if e.ID == id {
			e.sent = true
		}
	}
	return nil
}

func (s *Store) MarkAlertEmailFailed(_ context.Context, id kernel.ID, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.emails {
		if e.ID == id {
			e.Attempts++
		}
	}
	return nil
}

// Emails returns queued alert e-mails (sent or not).
func (s *Store) Emails() []usecase.AlertEmail {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []usecase.AlertEmail{}
	for _, e := range s.emails {
		out = append(out, e.AlertEmail)
	}
	return out
}
