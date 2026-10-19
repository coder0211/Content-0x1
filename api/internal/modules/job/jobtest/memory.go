package jobtest

import (
	"context"
	"encoding/json"
	"maps"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/be/acs/api/internal/modules/job/usecase"
	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Memory is an in-memory usecase.Repository that also implements
// ports.TxManager with snapshot/rollback semantics, so engine tests observe
// the same atomicity as Postgres (a failed transaction leaves no trace).
// Row locks are emulated by one global mutex held for the whole
// transaction; nested WithinTx calls reuse the outer transaction.
type Memory struct {
	txMu sync.Mutex // serialises transactions
	mu   sync.Mutex // guards the maps (non-transactional calls)

	jobs    map[kernel.ID]usecase.JobRecord
	steps   map[kernel.ID]usecase.StepRecord
	events  []usecase.EventRecord
	outbox  []usecase.OutboxRecord
	eventID int64
	boxID   int64
}

// NewMemory returns an empty store.
func NewMemory() *Memory {
	return &Memory{jobs: map[kernel.ID]usecase.JobRecord{}, steps: map[kernel.ID]usecase.StepRecord{}}
}

var (
	_ usecase.Repository = (*Memory)(nil)
	_ ports.TxManager    = (*Memory)(nil)
)

type txKey struct{}

type snapshot struct {
	jobs    map[kernel.ID]usecase.JobRecord
	steps   map[kernel.ID]usecase.StepRecord
	events  []usecase.EventRecord
	outbox  []usecase.OutboxRecord
	eventID int64
	boxID   int64
}

// WithinTx runs fn atomically: on error every change is rolled back.
func (m *Memory) WithinTx(ctx context.Context, fn func(context.Context) error) error {
	if ctx.Value(txKey{}) != nil {
		return fn(ctx)
	}
	m.txMu.Lock()
	defer m.txMu.Unlock()
	m.mu.Lock()
	snap := snapshot{maps.Clone(m.jobs), maps.Clone(m.steps), slices.Clone(m.events), slices.Clone(m.outbox), m.eventID, m.boxID}
	m.mu.Unlock()
	if err := fn(context.WithValue(ctx, txKey{}, true)); err != nil {
		m.mu.Lock()
		m.jobs, m.steps, m.events, m.outbox, m.eventID, m.boxID = snap.jobs, snap.steps, snap.events, snap.outbox, snap.eventID, snap.boxID
		m.mu.Unlock()
		return err
	}
	return nil
}

func (m *Memory) lock() func() { m.mu.Lock(); return m.mu.Unlock }

// InsertJob implements usecase.Repository.
func (m *Memory) InsertJob(_ context.Context, j *usecase.JobRecord) (bool, error) {
	defer m.lock()()
	if j.IdempotencyKey != "" {
		for _, o := range m.jobs {
			if o.CreatedBy == j.CreatedBy && o.IdempotencyKey == j.IdempotencyKey {
				return false, nil
			}
		}
	}
	m.jobs[j.ID] = *j
	return true, nil
}

// FindJobByIdempotency implements usecase.Repository.
func (m *Memory) FindJobByIdempotency(_ context.Context, createdBy kernel.ID, key string) (usecase.JobRecord, error) {
	defer m.lock()()
	for _, o := range m.jobs {
		if o.CreatedBy == createdBy && o.IdempotencyKey == key {
			return o, nil
		}
	}
	return usecase.JobRecord{}, kernel.ErrNotFound
}

// GetJob implements usecase.Repository.
func (m *Memory) GetJob(_ context.Context, id kernel.ID) (usecase.JobRecord, error) {
	defer m.lock()()
	j, ok := m.jobs[id]
	if !ok {
		return usecase.JobRecord{}, kernel.ErrNotFound
	}
	return j, nil
}

// LockJob implements usecase.Repository (the tx mutex is the lock).
func (m *Memory) LockJob(ctx context.Context, id kernel.ID) (usecase.JobRecord, error) {
	return m.GetJob(ctx, id)
}

// UpdateJob implements usecase.Repository.
func (m *Memory) UpdateJob(_ context.Context, j *usecase.JobRecord) error {
	defer m.lock()()
	if _, ok := m.jobs[j.ID]; !ok {
		return kernel.ErrNotFound
	}
	m.jobs[j.ID] = *j
	return nil
}

// RaiseJobProgress implements usecase.Repository.
func (m *Memory) RaiseJobProgress(_ context.Context, id kernel.ID, pct int) error {
	defer m.lock()()
	j, ok := m.jobs[id]
	if ok && !j.Status.IsTerminal() && pct > j.Progress {
		j.Progress = min(100, pct)
		m.jobs[id] = j
	}
	return nil
}

// ListJobs implements usecase.Repository.
func (m *Memory) ListJobs(_ context.Context, f usecase.JobFilter, limit int, after *kernel.TimeIDCursor) ([]usecase.JobRecord, error) {
	defer m.lock()()
	var out []usecase.JobRecord
	for _, j := range m.jobs {
		switch {
		case len(f.Status) > 0 && !slices.Contains(f.Status, j.Status),
			len(f.Type) > 0 && !slices.Contains(f.Type, j.Type),
			f.CampaignID != nil && (j.CampaignID == nil || *j.CampaignID != *f.CampaignID),
			f.CreatedBy != nil && j.CreatedBy != *f.CreatedBy,
			f.ProviderCode != "" && j.ProviderCode != f.ProviderCode,
			f.NeedsReconciliation != nil && j.NeedsReconciliation != *f.NeedsReconciliation:
			continue
		}
		out = append(out, j)
	}
	sort.Slice(out, func(a, b int) bool {
		if !out[a].CreatedAt.Equal(out[b].CreatedAt) {
			return out[a].CreatedAt.After(out[b].CreatedAt)
		}
		return out[a].ID.String() > out[b].ID.String()
	})
	if after != nil {
		i := 0
		for i < len(out) && (out[i].CreatedAt.After(after.At) || (out[i].CreatedAt.Equal(after.At) && out[i].ID.String() >= after.ID.String())) {
			i++
		}
		out = out[i:]
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// InsertStep implements usecase.Repository.
func (m *Memory) InsertStep(_ context.Context, s *usecase.StepRecord) error {
	defer m.lock()()
	for _, o := range m.steps {
		if o.JobID == s.JobID && o.Key == s.Key {
			return kernel.ErrConflict
		}
	}
	m.steps[s.ID] = *s
	return nil
}

// ListSteps implements usecase.Repository.
func (m *Memory) ListSteps(_ context.Context, jobID kernel.ID) ([]usecase.StepRecord, error) {
	defer m.lock()()
	return m.stepsOf(jobID), nil
}

func (m *Memory) stepsOf(jobID kernel.ID) []usecase.StepRecord {
	out := []usecase.StepRecord{}
	for _, s := range m.steps {
		if s.JobID == jobID {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a].CreatedAt.Before(out[b].CreatedAt) })
	return out
}

// GetStep implements usecase.Repository.
func (m *Memory) GetStep(_ context.Context, id kernel.ID) (usecase.StepRecord, error) {
	defer m.lock()()
	s, ok := m.steps[id]
	if !ok {
		return usecase.StepRecord{}, kernel.ErrNotFound
	}
	return s, nil
}

// UpdateStep implements usecase.Repository.
func (m *Memory) UpdateStep(_ context.Context, s *usecase.StepRecord) error {
	defer m.lock()()
	if _, ok := m.steps[s.ID]; !ok {
		return kernel.ErrNotFound
	}
	m.steps[s.ID] = *s
	return nil
}

// ExtendLease implements usecase.Repository.
func (m *Memory) ExtendLease(_ context.Context, stepID kernel.ID, owner string, until time.Time) (bool, error) {
	defer m.lock()()
	s, ok := m.steps[stepID]
	if !ok || s.Status != usecase.StepRunning || s.LeaseOwner != owner {
		return false, nil
	}
	s.LeaseExpiresAt = &until
	m.steps[stepID] = s
	return true, nil
}

// SaveStepOutput implements usecase.Repository.
func (m *Memory) SaveStepOutput(_ context.Context, stepID kernel.ID, owner string, output json.RawMessage) (bool, error) {
	defer m.lock()()
	s, ok := m.steps[stepID]
	if !ok || s.Status != usecase.StepRunning || s.LeaseOwner != owner {
		return false, nil
	}
	s.Output = output
	m.steps[stepID] = s
	return true, nil
}

// AppendEvent implements usecase.Repository.
func (m *Memory) AppendEvent(_ context.Context, e *usecase.EventRecord) error {
	defer m.lock()()
	m.eventID++
	e.ID = m.eventID
	m.events = append(m.events, *e)
	return nil
}

// ListEvents implements usecase.Repository.
func (m *Memory) ListEvents(_ context.Context, jobID kernel.ID, limit int) ([]usecase.EventRecord, error) {
	defer m.lock()()
	var out []usecase.EventRecord
	for _, e := range m.events {
		if e.JobID == jobID {
			out = append(out, e)
		}
	}
	if len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out, nil
}

// InsertOutbox implements usecase.Repository.
func (m *Memory) InsertOutbox(_ context.Context, o *usecase.OutboxRecord) error {
	defer m.lock()()
	m.boxID++
	o.ID = m.boxID
	m.outbox = append(m.outbox, *o)
	return nil
}

// ClaimOutbox implements usecase.Repository.
func (m *Memory) ClaimOutbox(_ context.Context, now time.Time, limit int) ([]usecase.OutboxRecord, error) {
	defer m.lock()()
	var out []usecase.OutboxRecord
	for _, o := range m.outbox {
		if o.PublishedAt == nil && !o.AvailableAt.After(now) {
			out = append(out, o)
		}
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].AvailableAt.Before(out[b].AvailableAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// MarkOutboxPublished implements usecase.Repository.
func (m *Memory) MarkOutboxPublished(_ context.Context, ids []int64, at time.Time) error {
	defer m.lock()()
	for i := range m.outbox {
		if slices.Contains(ids, m.outbox[i].ID) {
			t := at
			m.outbox[i].PublishedAt = &t
		}
	}
	return nil
}

// OldestUnpublishedOutbox implements usecase.Repository.
func (m *Memory) OldestUnpublishedOutbox(_ context.Context, now time.Time) (*time.Time, error) {
	defer m.lock()()
	oldest := now
	for _, o := range m.outbox {
		if o.PublishedAt == nil && o.AvailableAt.Before(oldest) {
			oldest = o.AvailableAt
		}
	}
	return &oldest, nil
}

// DeletePublishedOutbox implements usecase.Repository.
func (m *Memory) DeletePublishedOutbox(_ context.Context, before time.Time) (int64, error) {
	defer m.lock()()
	n := int64(0)
	keep := m.outbox[:0:0]
	for _, o := range m.outbox {
		if o.PublishedAt != nil && o.PublishedAt.Before(before) {
			n++
			continue
		}
		keep = append(keep, o)
	}
	m.outbox = keep
	return n, nil
}

func (m *Memory) stepsWhere(limit int, pred func(usecase.StepRecord) bool) []usecase.StepRecord {
	defer m.lock()()
	var out []usecase.StepRecord
	for _, s := range m.steps {
		if pred(s) {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a].CreatedAt.Before(out[b].CreatedAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// ListExpiredLeases implements usecase.Repository.
func (m *Memory) ListExpiredLeases(_ context.Context, now time.Time, limit int) ([]usecase.StepRecord, error) {
	return m.stepsWhere(limit, func(s usecase.StepRecord) bool {
		return s.Status == usecase.StepRunning && s.LeaseExpiresAt != nil && s.LeaseExpiresAt.Before(now)
	}), nil
}

// ListOverdueWaiting implements usecase.Repository.
func (m *Memory) ListOverdueWaiting(_ context.Context, before time.Time, limit int) ([]usecase.StepRecord, error) {
	return m.stepsWhere(limit, func(s usecase.StepRecord) bool {
		due := s.CreatedAt
		if s.NextAttemptAt != nil {
			due = *s.NextAttemptAt
		}
		return s.Status == usecase.StepWaitingProvider && due.Before(before)
	}), nil
}

// ListStaleQueued implements usecase.Repository.
func (m *Memory) ListStaleQueued(_ context.Context, before time.Time, limit int) ([]usecase.StepRecord, error) {
	m.mu.Lock()
	pending := map[kernel.ID]bool{}
	for _, o := range m.outbox {
		if o.PublishedAt == nil {
			var p usecase.TaskPayload
			if json.Unmarshal(o.Payload, &p) == nil {
				pending[p.StepID] = true
			}
		}
	}
	m.mu.Unlock()
	return m.stepsWhere(limit, func(s usecase.StepRecord) bool {
		due := s.CreatedAt
		if s.NextAttemptAt != nil {
			due = *s.NextAttemptAt
		}
		return s.Status == usecase.StepQueued && due.Before(before) && !pending[s.ID]
	}), nil
}

// CountNeedsReconciliation implements usecase.Repository.
func (m *Memory) CountNeedsReconciliation(context.Context) (int, error) {
	defer m.lock()()
	n := 0
	for _, j := range m.jobs {
		if j.NeedsReconciliation {
			n++
		}
	}
	return n, nil
}

// TryLock implements usecase.Repository (always granted).
func (m *Memory) TryLock(context.Context, int64) (bool, error) { return true, nil }

// Events returns a copy of all ai_job_events rows (test helper).
func (m *Memory) Events() []usecase.EventRecord {
	defer m.lock()()
	return slices.Clone(m.events)
}

// Outbox returns a copy of all outbox rows (test helper).
func (m *Memory) Outbox() []usecase.OutboxRecord {
	defer m.lock()()
	return slices.Clone(m.outbox)
}

// SetStep overwrites a step (test helper, e.g. to expire a lease).
func (m *Memory) SetStep(s usecase.StepRecord) {
	defer m.lock()()
	m.steps[s.ID] = s
}

// Step returns a step by job and key (test helper).
func (m *Memory) Step(jobID kernel.ID, key string) (usecase.StepRecord, bool) {
	defer m.lock()()
	for _, s := range m.stepsOf(jobID) {
		if s.Key == key {
			return s, true
		}
	}
	return usecase.StepRecord{}, false
}
