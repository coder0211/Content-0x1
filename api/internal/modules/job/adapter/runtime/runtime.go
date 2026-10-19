// Package runtime adapts the job engine to its drivers: the asynq queue
// (producer + consumer), a Redis fixed-window rate limiter and Prometheus
// metrics. The Redis semaphore of platform/redis is used as-is.
package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/be/acs/api/internal/modules/job/usecase"
	"github.com/be/acs/api/internal/platform/metrics"
	"github.com/be/acs/api/internal/platform/queue"
)

// Enqueuer is the producer side of platform/queue.
type Enqueuer interface {
	Enqueue(ctx context.Context, t queue.Task) error
}

// Queue implements usecase.TaskQueue on asynq.
type Queue struct{ C Enqueuer }

var _ usecase.TaskQueue = Queue{}

// Enqueue submits t; a duplicate task id means it is already queued.
func (q Queue) Enqueue(ctx context.Context, t usecase.Task) error {
	err := q.C.Enqueue(ctx, queue.Task{
		Queue: t.Queue, Type: usecase.TaskTypeStep, Payload: t.Payload, ID: t.ID, ProcessIn: t.ProcessIn, Timeout: t.Timeout,
	})
	if errors.Is(err, queue.ErrDuplicate) {
		return nil
	}
	return err
}

// Executor runs one step task (usecase.Service.Execute).
type Executor interface {
	Execute(ctx context.Context, p usecase.TaskPayload) error
}

// HandleTask decodes a queue task and executes it. Malformed payloads are
// dropped (logged); errors mark the asynq task failed and the reconciler
// recovers the step through its lease.
func HandleTask(exec Executor, log *slog.Logger) queue.HandlerFunc {
	return func(ctx context.Context, t queue.Task) error {
		var p usecase.TaskPayload
		if err := json.Unmarshal(t.Payload, &p); err != nil {
			log.ErrorContext(ctx, "drop malformed job task", slog.String("task_id", t.ID), slog.String("error", err.Error()))
			return nil //nolint:nilerr // retrying a malformed payload cannot succeed
		}
		if err := exec.Execute(ctx, p); err != nil {
			return fmt.Errorf("execute step %s: %w", p.StepID, err)
		}
		return nil
	}
}

// RateLimiter is a Redis fixed-window counter (usecase.RateLimiter).
type RateLimiter struct{ C *goredis.Client }

var _ usecase.RateLimiter = RateLimiter{}

var allowScript = goredis.NewScript(`
local n = redis.call('INCR', KEYS[1])
if n == 1 then redis.call('PEXPIRE', KEYS[1], ARGV[1]) end
local ttl = redis.call('PTTL', KEYS[1])
return {n, ttl}`)

// Allow consumes one unit of key in the current window.
func (l RateLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, time.Duration, error) {
	res, err := allowScript.Run(ctx, l.C, []string{"ratelimit:" + key}, window.Milliseconds()).Int64Slice()
	if err != nil {
		return true, 0, err
	}
	if res[0] > int64(limit) {
		return false, time.Duration(max(res[1], 0)) * time.Millisecond, nil
	}
	return true, 0, nil
}

// Metrics records engine measurements on the Prometheus collectors.
type Metrics struct{ M *metrics.Metrics }

var _ usecase.Metrics = Metrics{}

func (m Metrics) OutboxLag(seconds float64) { m.M.OutboxLag.Set(seconds) }
func (m Metrics) JobFinished(jobType, status string) {
	m.M.AIJobTotal.WithLabelValues(jobType, status).Inc()
}
func (m Metrics) StepDuration(stepType, provider string, d time.Duration) {
	m.M.AIStepDuration.WithLabelValues(stepType, provider).Observe(d.Seconds())
}
func (m Metrics) ProviderError(provider, kind string) {
	m.M.AIProviderErrors.WithLabelValues(provider, kind).Inc()
}
