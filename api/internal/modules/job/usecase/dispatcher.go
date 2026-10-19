package usecase

import (
	"context"
	"encoding/json"
	"log/slog"
	"slices"
	"time"
)

// DispatchBatch is the number of outbox rows claimed per round.
const DispatchBatch = 100

// DispatchOnce publishes due outbox rows to the queue (ACS-012). Rows are
// claimed with FOR UPDATE SKIP LOCKED so several dispatchers can run; a row
// is marked published only after a successful (or duplicate) enqueue, so a
// Redis outage loses nothing — the rows are simply retried next round.
func (s *Service) DispatchOnce(ctx context.Context) (int, error) {
	if s.Queue == nil {
		return 0, nil
	}
	n := 0
	err := s.Tx.WithinTx(ctx, func(ctx context.Context) error {
		now := s.now()
		rows, err := s.Repo.ClaimOutbox(ctx, now, DispatchBatch)
		if err != nil {
			return err
		}
		done := make([]int64, 0, len(rows))
		var enqErr error
		for _, row := range rows {
			var p TaskPayload
			if err := json.Unmarshal(row.Payload, &p); err != nil {
				s.log().ErrorContext(ctx, "drop malformed outbox row", slog.Int64("outbox_id", row.ID))
				done = append(done, row.ID)
				continue
			}
			queue, timeout := s.route(row.Topic, p.StepType)
			if err := s.Queue.Enqueue(ctx, Task{Queue: queue, ID: TaskID(p, row.ID), Payload: row.Payload, Timeout: timeout}); err != nil {
				enqErr = err
				break
			}
			done = append(done, row.ID)
		}
		n = len(done)
		if len(done) > 0 {
			if err := s.Repo.MarkOutboxPublished(ctx, done, now); err != nil {
				return err
			}
		}
		if enqErr != nil {
			s.log().WarnContext(ctx, "outbox enqueue failed; will retry", slog.String("error", enqErr.Error()))
		}
		return nil
	})
	if err != nil {
		return n, err
	}
	s.reportOutboxLag(ctx)
	return n, nil
}

// route picks the queue and asynq timeout for a step type.
func (s *Service) route(topic, stepType string) (string, time.Duration) {
	const fallbackTimeout = 35 * time.Minute
	if s.Steps != nil {
		if h, ok := s.Steps.Lookup(stepType); ok {
			return string(h.Queue()), h.Timeout() + 2*time.Minute
		}
	}
	if slices.Contains(AllQueues, Queue(topic)) {
		return topic, fallbackTimeout
	}
	// Unknown here: the consumer fails the step with STEP_TYPE_UNKNOWN.
	return string(QueueMaintenance), fallbackTimeout
}

func (s *Service) reportOutboxLag(ctx context.Context) {
	if s.Metrics == nil {
		return
	}
	now := s.now()
	oldest, err := s.Repo.OldestUnpublishedOutbox(ctx, now)
	if err != nil {
		return
	}
	lag := 0.0
	if oldest != nil {
		lag = max(0, now.Sub(*oldest).Seconds())
	}
	s.Metrics.OutboxLag(lag)
}

// RunDispatcher polls the outbox every interval (1s) until ctx ends.
func (s *Service) RunDispatcher(ctx context.Context, interval time.Duration) error {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		n, err := s.DispatchOnce(ctx)
		if err != nil && ctx.Err() == nil {
			s.log().WarnContext(ctx, "dispatch round failed", slog.String("error", err.Error()))
		}
		if n == DispatchBatch {
			continue // backlog: go again immediately
		}
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
		}
	}
}
