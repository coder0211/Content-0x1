package job

import (
	"testing"

	"github.com/be/acs/api/internal/modules/job/usecase"
	"github.com/be/acs/api/internal/platform/queue"
)

func TestQueueNamesMatchPlatform(t *testing.T) {
	if len(usecase.AllQueues) != len(queue.DefaultWeights) {
		t.Fatal("queue sets differ")
	}
	for _, q := range usecase.AllQueues {
		if _, ok := queue.DefaultWeights[string(q)]; !ok {
			t.Fatalf("queue %s has no platform weight", q)
		}
	}
}
