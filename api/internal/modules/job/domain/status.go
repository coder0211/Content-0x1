// Package domain holds the job engine's pure rules: job/step state
// machines, DAG validation and readiness, retry backoff and progress.
package domain

import "github.com/be/acs/api/internal/shared/kernel"

// JobStatus mirrors ai_jobs.status.
type JobStatus string

// Job statuses (SPEC-02 §5.2).
const (
	JobQueued    JobStatus = "queued"
	JobRunning   JobStatus = "running"
	JobSucceeded JobStatus = "succeeded"
	JobFailed    JobStatus = "failed"
	JobCancelled JobStatus = "cancelled"
)

// IsTerminal reports whether no further transition is possible (except a
// retry of a failed job).
func (s JobStatus) IsTerminal() bool {
	return s == JobSucceeded || s == JobFailed || s == JobCancelled
}

// StepStatus mirrors ai_job_steps.status.
type StepStatus string

// Step statuses.
const (
	StepPending         StepStatus = "pending"
	StepQueued          StepStatus = "queued"
	StepRunning         StepStatus = "running"
	StepWaitingProvider StepStatus = "waiting_provider"
	StepSucceeded       StepStatus = "succeeded"
	StepFailed          StepStatus = "failed"
	StepCancelled       StepStatus = "cancelled"
	StepSkipped         StepStatus = "skipped"
)

// IsTerminal reports whether the step will not run again without a retry.
func (s StepStatus) IsTerminal() bool {
	return s == StepSucceeded || s == StepFailed || s == StepCancelled || s == StepSkipped
}

// IsDone reports whether the step counts as completed for dependents and
// progress (succeeded or deliberately skipped).
func (s StepStatus) IsDone() bool { return s == StepSucceeded || s == StepSkipped }

// IsActive reports whether a worker owns or will own the step soon.
func (s StepStatus) IsActive() bool {
	return s == StepQueued || s == StepRunning || s == StepWaitingProvider
}

// JobTransitions is the job state machine. failed → queued is the retry
// path (POST /jobs/{id}/retry).
var JobTransitions = map[JobStatus][]JobStatus{
	JobQueued:    {JobRunning, JobCancelled, JobFailed},
	JobRunning:   {JobSucceeded, JobFailed, JobCancelled},
	JobFailed:    {JobQueued},
	JobSucceeded: {},
	JobCancelled: {},
}

// StepTransitions is the step state machine.
//   - running → queued: retry scheduled or expired lease requeued
//   - running → waiting_provider → running: asynchronous provider polling
//   - failed/cancelled/skipped → pending: job retry
var StepTransitions = map[StepStatus][]StepStatus{
	StepPending:         {StepQueued, StepSkipped, StepCancelled},
	StepQueued:          {StepRunning, StepCancelled, StepFailed, StepSkipped},
	StepRunning:         {StepSucceeded, StepFailed, StepQueued, StepWaitingProvider, StepCancelled},
	StepWaitingProvider: {StepRunning, StepCancelled, StepFailed},
	StepSucceeded:       {},
	StepFailed:          {StepPending},
	StepCancelled:       {StepPending},
	StepSkipped:         {StepPending},
}

func contains[T comparable](xs []T, x T) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

// CheckJobTransition returns INVALID_STATE_TRANSITION unless from → to is
// allowed.
func CheckJobTransition(from, to JobStatus) error {
	if next, ok := JobTransitions[from]; ok && contains(next, to) {
		return nil
	}
	return kernel.InvalidTransition(string(from), string(to))
}

// CheckStepTransition returns INVALID_STATE_TRANSITION unless from → to is
// allowed.
func CheckStepTransition(from, to StepStatus) error {
	if next, ok := StepTransitions[from]; ok && contains(next, to) {
		return nil
	}
	return kernel.InvalidTransition(string(from), string(to))
}
