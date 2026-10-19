package ports

import (
	"context"
	"encoding/json"

	"github.com/be/acs/api/internal/shared/kernel"
)

// Job event types streamed over SSE (SPEC-04 §3).
const (
	JobEventStatus   = "status"
	JobEventProgress = "progress"
	JobEventStep     = "step"
	JobEventDone     = "done"
)

// JobEvent is a progress notification for one job. Data is the JSON payload
// sent verbatim as the SSE data line.
type JobEvent struct {
	JobID kernel.ID       `json:"job_id"`
	Type  string          `json:"type"`
	Data  json.RawMessage `json:"data"`
}

// EventPublisher publishes job events (Redis pub/sub channel job:{id}).
// Publish only after the DB transaction that produced the change commits.
type EventPublisher interface {
	Publish(ctx context.Context, ev JobEvent) error
}

// EventSubscriber streams events for one job until ctx is cancelled or the
// returned close function is called.
type EventSubscriber interface {
	Subscribe(ctx context.Context, jobID kernel.ID) (<-chan JobEvent, func(), error)
}
