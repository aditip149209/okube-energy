package scheduler

import (
	"testing"

	"github.com/aditip149209/okube/pkg/node"
	"github.com/aditip149209/okube/pkg/task"
)

func TestComposableSchedulerDisableEnergyScoreSkipsEnergyStage(t *testing.T) {
	s := NewPipelineScheduler("roundrobin", "kahn")
	nodes := []*node.Node{
		{Name: "node-a", CarbonIntensity: node.CarbonOf(0.9)},
		{Name: "node-b", CarbonIntensity: node.CarbonOf(0.1)},
	}

	scores := s.Score(task.Task{}, nodes, &ScoreContext{DisableEnergyScore: true})

	// roundRobinResourceScorer assigns 0.1 to exactly one node and 1.0 to the
	// rest; with energy scoring disabled those raw resource scores must pass
	// through unchanged regardless of CarbonIntensity.
	seenLow, seenHigh := false, false
	for _, v := range scores {
		switch v {
		case 0.1:
			seenLow = true
		case 1.0:
			seenHigh = true
		default:
			t.Fatalf("expected raw resource scores (0.1/1.0) with energy scoring disabled, got %v", v)
		}
	}
	if !seenLow || !seenHigh {
		t.Fatalf("expected both raw resource score values present, got %v", scores)
	}
}

func TestComposableSchedulerEnergyScoreEnabledByDefault(t *testing.T) {
	s := NewPipelineScheduler("roundrobin", "kahn")
	nodes := []*node.Node{
		{Name: "node-a", CarbonIntensity: node.CarbonOf(0.9)},
		{Name: "node-b", CarbonIntensity: node.CarbonOf(0.1)},
	}

	scores := s.Score(task.Task{}, nodes, nil)

	for name, v := range scores {
		if v == 0.1 || v == 1.0 {
			t.Fatalf("expected energy scoring to shift raw resource score for %s away from 0.1/1.0, got %v", name, v)
		}
	}
}

func TestLeastLoadedScorerIsDeterministicAndPrefersIdleNodes(t *testing.T) {
	nodes := []*node.Node{
		{Name: "busy", TaskCount: 4},
		{Name: "idle", TaskCount: 0},
		{Name: "mid", TaskCount: 2},
	}
	s := NewPipelineScheduler("leastloaded", "")

	for i := 0; i < 3; i++ {
		scores := s.Score(task.Task{}, nodes, &ScoreContext{DisableEnergyScore: true})
		if got := s.Pick(scores, nodes).Name; got != "idle" {
			t.Fatalf("call %d: expected idle node, got %s (scores=%v)", i, got, scores)
		}
	}
}
