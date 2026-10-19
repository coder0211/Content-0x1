// Package redis wraps go-redis: connection, job event pub/sub (SSE source)
// and a counting semaphore used for provider/user concurrency caps.
package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Connect parses REDIS_URL and pings the server.
func Connect(ctx context.Context, url string) (*goredis.Client, error) {
	opt, err := goredis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("parse REDIS_URL: %w", err)
	}
	c := goredis.NewClient(opt)
	pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := c.Ping(pingCtx).Err(); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("ping redis: %w", err)
	}
	return c, nil
}

// JobChannel is the pub/sub channel for one job (SPEC-04 §3).
func JobChannel(jobID kernel.ID) string { return "job:" + jobID.String() }

// Events implements ports.EventPublisher and ports.EventSubscriber over
// Redis pub/sub. Delivery is best effort; clients fall back to polling.
type Events struct{ c *goredis.Client }

// NewEvents returns an Events bound to client.
func NewEvents(c *goredis.Client) *Events { return &Events{c: c} }

var (
	_ ports.EventPublisher  = (*Events)(nil)
	_ ports.EventSubscriber = (*Events)(nil)
)

// Publish sends ev on job:{id}.
func (e *Events) Publish(ctx context.Context, ev ports.JobEvent) error {
	b, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	return e.c.Publish(ctx, JobChannel(ev.JobID), b).Err()
}

// Subscribe streams events for jobID until ctx ends or close is called.
func (e *Events) Subscribe(ctx context.Context, jobID kernel.ID) (<-chan ports.JobEvent, func(), error) {
	sub := e.c.Subscribe(ctx, JobChannel(jobID))
	if _, err := sub.Receive(ctx); err != nil {
		_ = sub.Close()
		return nil, nil, err
	}
	out := make(chan ports.JobEvent, 16)
	done := make(chan struct{})
	go func() {
		defer close(out)
		ch := sub.Channel()
		for {
			select {
			case <-ctx.Done():
				return
			case <-done:
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				var ev ports.JobEvent
				if json.Unmarshal([]byte(msg.Payload), &ev) == nil {
					select {
					case out <- ev:
					default: // slow consumer: drop, the client re-syncs via GET /jobs/{id}
					}
				}
			}
		}
	}()
	closed := false
	return out, func() {
		if !closed {
			closed = true
			close(done)
			_ = sub.Close()
		}
	}, nil
}

// Semaphore is a lease-based counting semaphore (sorted set of holders
// scored by expiry), safe against crashed holders.
type Semaphore struct{ c *goredis.Client }

// NewSemaphore returns a Semaphore.
func NewSemaphore(c *goredis.Client) *Semaphore { return &Semaphore{c: c} }

var acquireScript = goredis.NewScript(`
local key, holder, limit, now, ttl = KEYS[1], ARGV[1], tonumber(ARGV[2]), tonumber(ARGV[3]), tonumber(ARGV[4])
redis.call('ZREMRANGEBYSCORE', key, '-inf', now)
if redis.call('ZSCORE', key, holder) then
  redis.call('ZADD', key, now + ttl, holder)
  return 1
end
if redis.call('ZCARD', key) < limit then
  redis.call('ZADD', key, now + ttl, holder)
  redis.call('PEXPIRE', key, ttl)
  return 1
end
return 0`)

// TryAcquire takes one slot of key for holder for ttl. It returns false when
// limit slots are taken; re-acquiring by the same holder extends its lease.
func (s *Semaphore) TryAcquire(ctx context.Context, key, holder string, limit int, ttl time.Duration) (bool, error) {
	now := time.Now().UnixMilli()
	n, err := acquireScript.Run(ctx, s.c, []string{"sem:" + key}, holder, limit, now, ttl.Milliseconds()).Int()
	return n == 1, err
}

// Release frees holder's slot.
func (s *Semaphore) Release(ctx context.Context, key, holder string) error {
	return s.c.ZRem(ctx, "sem:"+key, holder).Err()
}
