// Package queue wraps asynq. The job module is the only user: its dispatcher
// enqueues step tasks from the outbox and its worker runtime consumes them.
package queue

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"
)

// Queue names and asynq priority weights (SPEC-02 §5.4). The job module
// re-exports the names as typed constants.
const (
	Text        = "text"
	Image       = "image"
	Video       = "video"
	Render      = "render"
	Maintenance = "maintenance"
)

// DefaultWeights are the asynq priority weights per queue.
var DefaultWeights = map[string]int{Text: 6, Image: 3, Video: 1, Render: 3, Maintenance: 1}

// Task is one unit of work.
type Task struct {
	Queue   string
	Type    string
	Payload []byte
	// ID makes enqueue idempotent (asynq unique task id), e.g. step_id+attempt.
	ID        string
	ProcessIn time.Duration
	Timeout   time.Duration
	// Retention keeps completed task IDs so a duplicate enqueue is rejected.
	Retention time.Duration
}

// ErrDuplicate is returned when a task with the same ID already exists.
var ErrDuplicate = asynq.ErrTaskIDConflict

// Client enqueues tasks.
type Client struct{ c *asynq.Client }

// NewClient connects to Redis at url.
func NewClient(url string) (*Client, error) {
	opt, err := asynq.ParseRedisURI(url)
	if err != nil {
		return nil, fmt.Errorf("parse REDIS_URL: %w", err)
	}
	return &Client{c: asynq.NewClient(opt)}, nil
}

// Close releases the connection.
func (c *Client) Close() error { return c.c.Close() }

// Enqueue submits t. Retries are owned by the job engine, so asynq's own
// retry is disabled (MaxRetry 0).
func (c *Client) Enqueue(ctx context.Context, t Task) error {
	opts := []asynq.Option{asynq.Queue(t.Queue), asynq.MaxRetry(0)}
	if t.ID != "" {
		opts = append(opts, asynq.TaskID(t.ID))
	}
	if t.ProcessIn > 0 {
		opts = append(opts, asynq.ProcessIn(t.ProcessIn))
	}
	if t.Timeout > 0 {
		opts = append(opts, asynq.Timeout(t.Timeout))
	}
	if t.Retention > 0 {
		opts = append(opts, asynq.Retention(t.Retention))
	}
	_, err := c.c.EnqueueContext(ctx, asynq.NewTask(t.Type, t.Payload), opts...)
	return err
}

// HandlerFunc processes one task. Returning an error marks the asynq task
// failed (it is not retried by asynq; the job engine decides).
type HandlerFunc func(ctx context.Context, t Task) error

// Server consumes a subset of queues.
type Server struct {
	srv *asynq.Server
	mux *asynq.ServeMux
}

// ServerConfig configures a Server.
type ServerConfig struct {
	RedisURL    string
	Queues      []string // subset of queue names to consume
	Concurrency int
	Logger      *slog.Logger
}

// NewServer builds a Server consuming cfg.Queues with DefaultWeights.
func NewServer(cfg ServerConfig) (*Server, error) {
	opt, err := asynq.ParseRedisURI(cfg.RedisURL)
	if err != nil {
		return nil, fmt.Errorf("parse REDIS_URL: %w", err)
	}
	queues := map[string]int{}
	for _, q := range cfg.Queues {
		w, ok := DefaultWeights[q]
		if !ok {
			return nil, fmt.Errorf("unknown queue %q", q)
		}
		queues[q] = w
	}
	srv := asynq.NewServer(opt, asynq.Config{
		Concurrency:    cfg.Concurrency,
		Queues:         queues,
		Logger:         slogAdapter{cfg.Logger},
		RetryDelayFunc: func(int, error, *asynq.Task) time.Duration { return 0 },
	})
	return &Server{srv: srv, mux: asynq.NewServeMux()}, nil
}

// Handle routes tasks of taskType to h.
func (s *Server) Handle(taskType string, h HandlerFunc) {
	s.mux.HandleFunc(taskType, func(ctx context.Context, t *asynq.Task) error {
		q, _ := asynq.GetQueueName(ctx)
		id, _ := asynq.GetTaskID(ctx)
		return h(ctx, Task{Queue: q, Type: t.Type(), Payload: t.Payload(), ID: id})
	})
}

// Run processes tasks until ctx is cancelled, then shuts down gracefully.
func (s *Server) Run(ctx context.Context) error {
	if err := s.srv.Start(s.mux); err != nil {
		return err
	}
	<-ctx.Done()
	s.srv.Shutdown()
	return nil
}

type slogAdapter struct{ l *slog.Logger }

func (a slogAdapter) log(level slog.Level, args ...any) {
	if a.l != nil {
		a.l.Log(context.Background(), level, fmt.Sprint(args...), "component", "asynq")
	}
}
func (a slogAdapter) Debug(args ...any) { a.log(slog.LevelDebug, args...) }
func (a slogAdapter) Info(args ...any)  { a.log(slog.LevelInfo, args...) }
func (a slogAdapter) Warn(args ...any)  { a.log(slog.LevelWarn, args...) }
func (a slogAdapter) Error(args ...any) { a.log(slog.LevelError, args...) }
func (a slogAdapter) Fatal(args ...any) { a.log(slog.LevelError, args...) }
