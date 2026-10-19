package domain

import (
	"errors"
	"testing"
	"time"

	"github.com/be/acs/api/internal/shared/kernel"
)

func TestJobTransitionTable(t *testing.T) {
	all := []JobStatus{JobQueued, JobRunning, JobSucceeded, JobFailed, JobCancelled}
	allowed := map[[2]JobStatus]bool{
		{JobQueued, JobRunning}: true, {JobQueued, JobCancelled}: true, {JobQueued, JobFailed}: true,
		{JobRunning, JobSucceeded}: true, {JobRunning, JobFailed}: true, {JobRunning, JobCancelled}: true,
		{JobFailed, JobQueued}: true,
	}
	for _, from := range all {
		for _, to := range all {
			err := CheckJobTransition(from, to)
			if allowed[[2]JobStatus{from, to}] {
				if err != nil {
					t.Errorf("%s→%s should be allowed: %v", from, to, err)
				}
				continue
			}
			if !errors.Is(err, kernel.ErrInvalidTransition) {
				t.Errorf("%s→%s should be INVALID_STATE_TRANSITION, got %v", from, to, err)
			}
		}
	}
}

func TestStepTransitionTable(t *testing.T) {
	all := []StepStatus{StepPending, StepQueued, StepRunning, StepWaitingProvider, StepSucceeded, StepFailed, StepCancelled, StepSkipped}
	n := 0
	for _, from := range all {
		for _, to := range all {
			err := CheckStepTransition(from, to)
			want := contains(StepTransitions[from], to)
			if want != (err == nil) {
				t.Errorf("%s→%s: allowed=%v err=%v", from, to, want, err)
			}
			if err != nil && !errors.Is(err, kernel.ErrInvalidTransition) {
				t.Errorf("%s→%s: wrong error %v", from, to, err)
			}
			if want {
				n++
			}
		}
	}
	if CheckStepTransition(StepSucceeded, StepPending) == nil {
		t.Fatal("succeeded is final")
	}
	if n == 0 {
		t.Fatal("no transitions")
	}
}

func TestValidateDAG(t *testing.T) {
	ok := []Node{{Key: "a", Type: "x"}, {Key: "b", Type: "x", DependsOn: []string{"a"}}}
	if err := ValidateDAG(ok); err != nil {
		t.Fatal(err)
	}
	for name, nodes := range map[string][]Node{
		"empty":   nil,
		"dup":     {{Key: "a", Type: "x"}, {Key: "a", Type: "x"}},
		"unknown": {{Key: "a", Type: "x", DependsOn: []string{"z"}}},
		"self":    {{Key: "a", Type: "x", DependsOn: []string{"a"}}},
		"cycle":   {{Key: "a", Type: "x", DependsOn: []string{"b"}}, {Key: "b", Type: "x", DependsOn: []string{"a"}}},
		"notype":  {{Key: "a"}},
	} {
		if err := ValidateDAG(nodes); !errors.Is(err, kernel.ErrValidation) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestReadyKeysParallelJoin(t *testing.T) {
	steps := []StepState{
		{Key: "a", Status: StepSucceeded},
		{Key: "b1", Status: StepPending, DependsOn: []string{"a"}},
		{Key: "b2", Status: StepPending, DependsOn: []string{"a"}},
		{Key: "b3", Status: StepSkipped, DependsOn: []string{"a"}},
		{Key: "j", Status: StepPending, DependsOn: []string{"b1", "b2", "b3"}},
	}
	got := ReadyKeys(steps)
	if len(got) != 2 || got[0] != "b1" || got[1] != "b2" {
		t.Fatalf("ready = %v", got)
	}
	steps[1].Status, steps[2].Status = StepSucceeded, StepSucceeded
	if got := ReadyKeys(steps); len(got) != 1 || got[0] != "j" {
		t.Fatalf("join ready = %v", got)
	}
	if AllDone(steps) {
		t.Fatal("join still pending")
	}
	if p := Progress(steps, 50); p != 90 {
		t.Fatalf("progress = %d", p)
	}
}

func TestBackoff(t *testing.T) {
	for _, tc := range []struct {
		n    int
		want time.Duration
	}{{1, 4 * time.Second}, {2, 8 * time.Second}, {4, 32 * time.Second}, {5, 60 * time.Second}, {10, 60 * time.Second}} {
		if got := Backoff(tc.n, 0); got != tc.want {
			t.Errorf("Backoff(%d) = %v want %v", tc.n, got, tc.want)
		}
		if got := Backoff(tc.n, 0.5); got != tc.want+500*time.Millisecond {
			t.Errorf("jitter: %v", got)
		}
	}
}
