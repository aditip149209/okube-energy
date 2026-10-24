package sim

import (
	"testing"

	"github.com/aditip149209/okube/pkg/appgroup"
	"github.com/aditip149209/okube/pkg/node"
	"github.com/aditip149209/okube/pkg/scheduler"
	"github.com/aditip149209/okube/pkg/task"
	"github.com/aditip149209/okube/pkg/topology"
)

// Smoke test: the real pipeline runs offline and the network and energy
// stages each move the pick as expected.
func TestPipelineRunsOffline(t *testing.T) {
	// db is already placed on n1. n1 is closest to it but dirtiest; n3 is
	// farthest but cleanest.
	nodes := func() []*node.Node {
		return []*node.Node{
			{Name: "n1", CarbonIntensity: node.CarbonOf(0.9)},
			{Name: "n2", CarbonIntensity: node.CarbonOf(0.5)},
			{Name: "n3", CarbonIntensity: node.CarbonOf(0.1)},
		}
	}
	topo := &topology.NetworkTopology{NodeLatencies: map[string]map[string]float64{
		"n1": {"n1": 0, "n2": 10, "n3": 40},
		"n2": {"n1": 10, "n2": 0, "n3": 20},
		"n3": {"n1": 40, "n2": 20, "n3": 0},
	}}
	ag, err := appgroup.NewAppGroup("app", []string{"db", "web"}, []appgroup.DependencyEdge{{From: "web", To: "db"}})
	if err != nil {
		t.Fatal(err)
	}
	filterCtx := &scheduler.FilterContext{
		AppGroup:                ag,
		NetworkTopology:         topo,
		DependencyNodeByService: map[string]string{"db": "n1"},
	}
	web := task.Task{Name: "web", AppID: "app", ServiceID: "web"}

	run := func(ctx *scheduler.ScoreContext) string {
		s := scheduler.NewPipelineScheduler("leastloaded", "")
		cands := s.SelectCandidateNodes(web, nodes(), filterCtx)
		if len(cands) != 3 {
			t.Fatalf("expected 3 candidates, got %d", len(cands))
		}
		scores := s.Score(web, cands, ctx)
		t.Logf("scores=%v", scores)
		return s.Pick(scores, cands).Name
	}

	w := 0.0
	big := 5.0
	networkOnly := run(&scheduler.ScoreContext{Filter: filterCtx, DisableEnergyScore: true})
	energyOnly := run(&scheduler.ScoreContext{Filter: filterCtx, DisableNetworkScore: true, EnergyWeight: &big})
	fusedZero := run(&scheduler.ScoreContext{Filter: filterCtx, EnergyWeight: &w})

	t.Logf("network-only=%s energy-only=%s fused(w=0)=%s", networkOnly, energyOnly, fusedZero)
	if networkOnly != "n1" {
		t.Errorf("network-only should co-locate with db on n1, got %s", networkOnly)
	}
	if energyOnly != "n3" {
		t.Errorf("energy-only should pick the cleanest node n3, got %s", energyOnly)
	}
	if fusedZero != networkOnly {
		t.Errorf("fused with w=0 should equal network-only, got %s vs %s", fusedZero, networkOnly)
	}
}

// Documents a finding: with the default round-robin resource scorer the
// network-optimal node loses, because round-robin hands one node 0.1 and the
// rest 1.0, which min-max normalises to a 0/100 swing at weight 0.3 -- larger
// than the network gain here (30 vs 17.5).
func TestRoundRobinResourceScoreOutweighsNetwork(t *testing.T) {
	topo := &topology.NetworkTopology{NodeLatencies: map[string]map[string]float64{
		"n1": {"n1": 0, "n2": 10, "n3": 40},
		"n2": {"n1": 10, "n2": 0, "n3": 20},
		"n3": {"n1": 40, "n2": 20, "n3": 0},
	}}
	ag, _ := appgroup.NewAppGroup("app", []string{"db", "web"}, []appgroup.DependencyEdge{{From: "web", To: "db"}})
	filterCtx := &scheduler.FilterContext{AppGroup: ag, NetworkTopology: topo, DependencyNodeByService: map[string]string{"db": "n1"}}
	web := task.Task{Name: "web", AppID: "app", ServiceID: "web"}
	nodes := []*node.Node{{Name: "n1"}, {Name: "n2"}, {Name: "n3"}}

	s := scheduler.NewPipelineScheduler("", "")
	scores := s.Score(web, nodes, &scheduler.ScoreContext{Filter: filterCtx, DisableEnergyScore: true})
	if got := s.Pick(scores, nodes).Name; got != "n2" {
		t.Fatalf("expected round-robin penalty to push the pick to n2, got %s (scores=%v)", got, scores)
	}
}
