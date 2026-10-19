package domain

import (
	"math"
	"strconv"
	"time"

	"github.com/be/acs/api/internal/shared/kernel"
)

// Node is one step of a job DAG.
type Node struct {
	Key       string
	Type      string
	DependsOn []string
}

// ValidateDAG checks keys are present and unique, dependencies exist and the
// graph has no cycle. Every problem is reported as a validation detail.
func ValidateDAG(nodes []Node) error {
	var v kernel.ValidationErrors
	if len(nodes) == 0 {
		v.Add("steps", "required")
		return v.Err()
	}
	idx := make(map[string]int, len(nodes))
	for i, n := range nodes {
		switch {
		case n.Key == "":
			v.Add(field(i, "key"), "required")
		case idx[n.Key] != 0:
			v.Add(field(i, "key"), "duplicate")
		default:
			idx[n.Key] = i + 1
		}
		if n.Type == "" {
			v.Add(field(i, "type"), "required")
		}
	}
	for i, n := range nodes {
		for _, d := range n.DependsOn {
			if idx[d] == 0 {
				v.Add(field(i, "depends_on"), "unknown_step:"+d)
			} else if d == n.Key {
				v.Add(field(i, "depends_on"), "self_dependency")
			}
		}
	}
	if err := v.Err(); err != nil {
		return err
	}
	// Kahn's algorithm: a remaining node means a cycle.
	indeg := make(map[string]int, len(nodes))
	children := map[string][]string{}
	for _, n := range nodes {
		indeg[n.Key] += 0
		for _, d := range n.DependsOn {
			indeg[n.Key]++
			children[d] = append(children[d], n.Key)
		}
	}
	queue := make([]string, 0, len(nodes))
	for _, n := range nodes {
		if indeg[n.Key] == 0 {
			queue = append(queue, n.Key)
		}
	}
	seen := 0
	for len(queue) > 0 {
		k := queue[0]
		queue = queue[1:]
		seen++
		for _, c := range children[k] {
			indeg[c]--
			if indeg[c] == 0 {
				queue = append(queue, c)
			}
		}
	}
	if seen != len(nodes) {
		v.Add("steps", "cycle")
	}
	return v.Err()
}

func field(i int, name string) string {
	return "steps[" + strconv.Itoa(i) + "]." + name
}

// StepState is the minimal view needed for readiness decisions.
type StepState struct {
	Key       string
	Status    StepStatus
	DependsOn []string
}

// ReadyKeys returns the keys of pending steps whose dependencies are all
// done (succeeded or skipped), in input order.
func ReadyKeys(steps []StepState) []string {
	status := make(map[string]StepStatus, len(steps))
	for _, s := range steps {
		status[s.Key] = s.Status
	}
	var out []string
	for _, s := range steps {
		if s.Status != StepPending {
			continue
		}
		ok := true
		for _, d := range s.DependsOn {
			if !status[d].IsDone() {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, s.Key)
		}
	}
	return out
}

// AllDone reports whether every step succeeded or was skipped.
func AllDone(steps []StepState) bool {
	for _, s := range steps {
		if !s.Status.IsDone() {
			return false
		}
	}
	return true
}

// Progress is the share of done steps plus the fraction reported by one
// running step, 0–100. A job only shows 100 when it succeeded.
func Progress(steps []StepState, runningPct int) int {
	if len(steps) == 0 {
		return 0
	}
	done := 0
	for _, s := range steps {
		if s.Status.IsDone() {
			done++
		}
	}
	runningPct = max(0, min(100, runningPct))
	p := (float64(done)*100 + float64(runningPct)) / float64(len(steps))
	return max(0, min(99, int(math.Floor(p))))
}

// Retry policy (SPEC-02 §5.3).
const (
	DefaultMaxAttempts      = 3
	DefaultVideoMaxAttempts = 5
	backoffBase             = 2 * time.Second
	backoffCap              = 60 * time.Second
)

// Backoff is min(2^n × 2s, 60s) + jitter where n is the 1-based attempt that
// just failed and jitter ∈ [0, 1s) comes from rnd ∈ [0, 1).
func Backoff(attempt int, rnd float64) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	d := backoffCap
	if attempt < 6 {
		d = min(backoffBase*time.Duration(1<<attempt), backoffCap)
	}
	rnd = max(0, min(rnd, 0.999999))
	return d + time.Duration(rnd*float64(time.Second))
}
