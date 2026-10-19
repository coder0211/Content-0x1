package usecase

import (
	"context"
	"log/slog"
	"time"
)

// ReconcileReport summarises one maintenance round.
type ReconcileReport struct {
	Skipped             bool // another instance holds the lock
	LeasesRequeued      int
	LeasesFailed        int
	WaitingPolled       int
	WaitingTimedOut     int
	QueuedRedispatched  int
	OutboxDeleted       int64
	NeedsReconciliation int
	UploadsPurged       int
}

const reconcileBatch = 200

// ReconcileOnce is the maintenance cron (ACS-018, every 5 minutes):
//   - running steps whose lease expired → requeued (next attempt) or failed
//   - waiting_provider steps whose poll is overdue → polled now (GetStatus
//     via the handler); waiting longer than MaxProviderWait → failed with
//     PROVIDER_WAIT_TIMEOUT and needs_reconciliation
//   - queued steps whose task vanished (Redis lost it) → re-dispatched
//   - published outbox rows older than 7 days → deleted
//   - jobs flagged needs_reconciliation → counted and logged for ops
//   - pending uploads older than 24h → purged through the file module
func (s *Service) ReconcileOnce(ctx context.Context) (ReconcileReport, error) {
	var rep ReconcileReport
	err := s.inTx(ctx, func(ctx context.Context, fx *effects) error {
		ok, err := s.Repo.TryLock(ctx, reconcilerAdvisoryLockID)
		if err != nil {
			return err
		}
		if !ok {
			rep.Skipped = true
			return nil
		}
		now := s.now()
		expired, err := s.Repo.ListExpiredLeases(ctx, now, reconcileBatch)
		if err != nil {
			return err
		}
		for _, st := range expired {
			if err := s.recoverLease(ctx, fx, st, &rep); err != nil {
				return err
			}
		}
		overdue, err := s.Repo.ListOverdueWaiting(ctx, now.Add(-durOr(s.OverdueGrace, DefaultOverdueGrace)), reconcileBatch)
		if err != nil {
			return err
		}
		for _, st := range overdue {
			if err := s.recoverWaiting(ctx, fx, st, &rep); err != nil {
				return err
			}
		}
		stale, err := s.Repo.ListStaleQueued(ctx, now.Add(-durOr(s.StaleQueuedAfter, DefaultStaleQueuedAfter)), reconcileBatch)
		if err != nil {
			return err
		}
		for i := range stale {
			// A new outbox row (unique task id) — the claim check prevents
			// double execution if the old task still exists.
			if err := s.enqueue(ctx, &stale[i], TaskDefer, stale[i].AttemptCount+1, now); err != nil {
				return err
			}
			rep.QueuedRedispatched++
		}
		if rep.OutboxDeleted, err = s.Repo.DeletePublishedOutbox(ctx, now.Add(-DefaultOutboxRetention)); err != nil {
			return err
		}
		if rep.NeedsReconciliation, err = s.Repo.CountNeedsReconciliation(ctx); err != nil {
			return err
		}
		return nil
	})
	if err != nil || rep.Skipped {
		return rep, err
	}
	if s.PurgeStaleUploads != nil {
		n, perr := s.PurgeStaleUploads(ctx, DefaultPendingUploadTTL)
		if perr != nil {
			s.log().WarnContext(ctx, "purge pending uploads failed", slog.String("error", perr.Error()))
		}
		rep.UploadsPurged = n
	}
	if rep.NeedsReconciliation > 0 {
		s.log().WarnContext(ctx, "jobs need cost reconciliation", slog.Int("count", rep.NeedsReconciliation))
	}
	s.log().InfoContext(ctx, "reconcile round", slog.Int("leases_requeued", rep.LeasesRequeued), slog.Int("leases_failed", rep.LeasesFailed),
		slog.Int("waiting_polled", rep.WaitingPolled), slog.Int("waiting_timed_out", rep.WaitingTimedOut),
		slog.Int("queued_redispatched", rep.QueuedRedispatched), slog.Int("uploads_purged", rep.UploadsPurged))
	return rep, nil
}

func (s *Service) recoverLease(ctx context.Context, fx *effects, cand StepRecord, rep *ReconcileReport) error {
	j, err := s.Repo.LockJob(ctx, cand.JobID)
	if err != nil {
		return err
	}
	st, err := s.Repo.GetStep(ctx, cand.ID)
	if err != nil {
		return err
	}
	now := s.now()
	if st.Status != StepRunning || st.LeaseExpiresAt == nil || st.LeaseExpiresAt.After(now) || j.Status.IsTerminal() {
		return nil
	}
	id := st.ID
	if err := s.appendEvent(ctx, j.ID, &id, EventLeaseExpired, map[string]any{"step_key": st.Key, "attempt": st.AttemptCount, "owner": st.LeaseOwner}); err != nil {
		return err
	}
	var h StepHandler
	if s.Steps != nil {
		h, _ = s.Steps.Lookup(st.Type)
	}
	st.ErrorKind, st.ErrorMessage = "LeaseExpired", "worker lease expired"
	if st.AttemptCount < maxAttempts(st, h) && !j.CancelRequested {
		st.NextAttemptAt = &now
		if err := s.setStepStatus(ctx, fx, &st, StepQueued); err != nil {
			return err
		}
		rep.LeasesRequeued++
		return s.enqueue(ctx, &st, TaskRun, st.AttemptCount+1, now)
	}
	steps, err := s.Repo.ListSteps(ctx, j.ID)
	if err != nil {
		return err
	}
	if err := s.setStepStatus(ctx, fx, &st, StepFailed); err != nil {
		return err
	}
	steps[indexOf(steps, st.ID)] = st
	rep.LeasesFailed++
	if j.CancelRequested {
		return s.finalize(ctx, fx, &j, steps, StatusCancelled, CodeCancelled, "Tác vụ đã bị hủy")
	}
	return s.finalize(ctx, fx, &j, steps, StatusFailed, CodeLeaseExpired, "Worker xử lý bị gián đoạn, vui lòng chạy lại")
}

func (s *Service) recoverWaiting(ctx context.Context, fx *effects, cand StepRecord, rep *ReconcileReport) error {
	j, err := s.Repo.LockJob(ctx, cand.JobID)
	if err != nil {
		return err
	}
	st, err := s.Repo.GetStep(ctx, cand.ID)
	if err != nil {
		return err
	}
	if st.Status != StepWaitingProvider || j.Status.IsTerminal() {
		return nil
	}
	now := s.now()
	id := st.ID
	if st.StartedAt != nil && now.Sub(*st.StartedAt) > durOr(s.MaxProviderWait, DefaultMaxProviderWait) && !j.CancelRequested {
		steps, err := s.Repo.ListSteps(ctx, j.ID)
		if err != nil {
			return err
		}
		st.ErrorKind, st.ErrorMessage = "Timeout", "provider did not finish in time"
		if err := s.setStepStatus(ctx, fx, &st, StepFailed); err != nil {
			return err
		}
		steps[indexOf(steps, st.ID)] = st
		j.NeedsReconciliation = true // the provider may have charged
		rep.WaitingTimedOut++
		return s.finalize(ctx, fx, &j, steps, StatusFailed, CodeProviderWaitTimeout, "Nhà cung cấp AI xử lý quá lâu, tác vụ đã dừng")
	}
	st.NextAttemptAt = &now
	if err := s.Repo.UpdateStep(ctx, &st); err != nil {
		return err
	}
	if err := s.appendEvent(ctx, j.ID, &id, EventReconciled, map[string]any{"step_key": st.Key, "action": "poll"}); err != nil {
		return err
	}
	rep.WaitingPolled++
	return s.enqueue(ctx, &st, TaskPoll, st.AttemptCount, now)
}

// RunReconciler runs ReconcileOnce every interval until ctx ends.
func (s *Service) RunReconciler(ctx context.Context, interval time.Duration) error {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		if _, err := s.ReconcileOnce(ctx); err != nil && ctx.Err() == nil {
			s.log().WarnContext(ctx, "reconcile round failed", slog.String("error", err.Error()))
		}
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
		}
	}
}
