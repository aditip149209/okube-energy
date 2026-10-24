package scheduler

import (
	"testing"

	"github.com/aditip149209/okube/pkg/node"
	"github.com/aditip149209/okube/pkg/task"
)

func TestApplyEnergyAwareScorePrefersLowerEnergyNode(t *testing.T) {
	baseScores := map[string]float64{
		"node-a": 1.0,
		"node-b": 1.0,
	}

	nodes := []*node.Node{
		{Name: "node-a", TaskCount: 5, Memory: 16000, MemoryAllocated: 12000, CarbonIntensity: node.CarbonOf(0.8)},
		{Name: "node-b", TaskCount: 1, Memory: 16000, MemoryAllocated: 3000, CarbonIntensity: node.CarbonOf(0.2)},
	}

	weight := 0.8
	scores := applyEnergyAwareScore(task.Task{Flexible: true}, nodes, baseScores, &ScoreContext{EnergyWeight: &weight})
	if scores["node-a"] <= scores["node-b"] {
		t.Fatalf("expected lower-energy node to win, got node-a=%v node-b=%v", scores["node-a"], scores["node-b"])
	}
}

func TestApplyEnergyAwareScoreReducesWeightForCriticalJobs(t *testing.T) {
	baseScores := map[string]float64{
		"node-a": 1.0,
		"node-b": 1.0,
	}

	nodes := []*node.Node{
		{Name: "node-a", TaskCount: 3, Memory: 16000, MemoryAllocated: 8000, CarbonIntensity: node.CarbonOf(0.8)},
		{Name: "node-b", TaskCount: 1, Memory: 16000, MemoryAllocated: 4000, CarbonIntensity: node.CarbonOf(0.2)},
	}

	weight := 0.8
	scores := applyEnergyAwareScore(task.Task{Criticality: "critical"}, nodes, baseScores, &ScoreContext{EnergyWeight: &weight})
	if scores["node-a"] <= scores["node-b"] {
		t.Fatalf("expected critical jobs to avoid over-weighting energy, got node-a=%v node-b=%v", scores["node-a"], scores["node-b"])
	}
}

// Regression test: an explicit EnergyWeight of 0 must actually zero out the
// energy contribution. Before the pointer fix, resolveEnergyWeight checked
// "> 0" so 0 was indistinguishable from "not set" and silently fell back to
// defaultEnergyWeight (0.2).
func TestApplyEnergyAwareScoreZeroWeightDisablesEnergyContribution(t *testing.T) {
	baseScores := map[string]float64{
		"node-a": 1.0,
		"node-b": 2.0,
	}
	nodes := []*node.Node{
		{Name: "node-a", CarbonIntensity: node.CarbonOf(0.9)},
		{Name: "node-b", CarbonIntensity: node.CarbonOf(0.1)},
	}

	zero := 0.0
	scores := applyEnergyAwareScore(task.Task{}, nodes, baseScores, &ScoreContext{EnergyWeight: &zero})
	if scores["node-a"] != baseScores["node-a"] || scores["node-b"] != baseScores["node-b"] {
		t.Fatalf("expected zero energy weight to leave base scores unchanged, got node-a=%v node-b=%v",
			scores["node-a"], scores["node-b"])
	}
}

func TestResolveEnergyWeightNilFallsBackToDefault(t *testing.T) {
	if w := resolveEnergyWeight(nil, &task.Task{}); w != defaultEnergyWeight {
		t.Fatalf("expected nil ScoreContext to fall back to default weight %v, got %v", defaultEnergyWeight, w)
	}
	if w := resolveEnergyWeight(&ScoreContext{}, &task.Task{}); w != defaultEnergyWeight {
		t.Fatalf("expected unset EnergyWeight to fall back to default weight %v, got %v", defaultEnergyWeight, w)
	}
}

// The energy term must have the same effective influence whether the incoming
// scores are on the 0-100 network scale or the raw 0.1/1.0 round-robin scale.
func TestApplyEnergyAwareScoreInfluenceIsScaleInvariant(t *testing.T) {
	nodes := []*node.Node{
		{Name: "node-a", CarbonIntensity: node.CarbonOf(0.8)},
		{Name: "node-b", CarbonIntensity: node.CarbonOf(0.2)},
	}
	weight := 0.5

	rawScale := applyEnergyAwareScore(task.Task{}, nodes, map[string]float64{"node-a": 0.1, "node-b": 1.0}, &ScoreContext{EnergyWeight: &weight})
	netScale := applyEnergyAwareScore(task.Task{}, nodes, map[string]float64{"node-a": 0, "node-b": 100}, &ScoreContext{EnergyWeight: &weight})

	for _, name := range []string{"node-a", "node-b"} {
		if d := rawScale[name] - netScale[name]; d > 1e-9 || d < -1e-9 {
			t.Fatalf("expected scale-invariant scores for %s, got raw=%v net=%v", name, rawScale[name], netScale[name])
		}
	}
}

func TestApplyEnergyAwareScoreEqualCarbonKeepsBaseRanking(t *testing.T) {
	nodes := []*node.Node{
		{Name: "node-a", CarbonIntensity: node.CarbonOf(0.5)},
		{Name: "node-b", CarbonIntensity: node.CarbonOf(0.5)},
	}
	scores := applyEnergyAwareScore(task.Task{Flexible: true}, nodes, map[string]float64{"node-a": 10, "node-b": 40}, nil)
	if scores["node-a"] >= scores["node-b"] {
		t.Fatalf("expected equal-carbon nodes to keep base ranking, got node-a=%v node-b=%v", scores["node-a"], scores["node-b"])
	}
}

// Task memory is bytes, node memory is KB: a task using 1 GiB on a 4 GiB node
// must add 0.25 to the load factor, not ~0.
func TestEstimateNodeEnergyCostConvertsMemoryUnits(t *testing.T) {
	n := &node.Node{Name: "n", Memory: 4 * 1024 * 1024, Cores: 4, CarbonIntensity: node.CarbonOf(1.0)}

	idle := estimateNodeEnergyCost(n, &task.Task{})
	loaded := estimateNodeEnergyCost(n, &task.Task{Memory: 1 << 30, Cpu: 2})

	// carbon 1.0 saturates the carbon term; cost = 100 * min(1, load/2).
	// idle load = 1; loaded load = 1 + 0.25 (mem) + 0.5 (cpu) = 1.75.
	if want := 50.0; idle != want {
		t.Fatalf("expected idle cost %v, got %v", want, idle)
	}
	if want := 87.5; loaded != want {
		t.Fatalf("expected loaded cost %v, got %v", want, loaded)
	}
}

// A real intensity of 0 is a fully low-carbon grid, not "unset": it must cost
// nothing, while a node with no data falls back to the default factor.
func TestEstimateNodeEnergyCostZeroCarbonIsFreeAndNilIsDefault(t *testing.T) {
	zero := &node.Node{Name: "zero", CarbonIntensity: node.CarbonOf(0)}
	if got := estimateNodeEnergyCost(zero, &task.Task{}); got != 0 {
		t.Fatalf("expected zero carbon to cost 0, got %v", got)
	}

	unknown := &node.Node{Name: "unknown"}
	atDefault := &node.Node{Name: "default", CarbonIntensity: node.CarbonOf(defaultCarbonFactor)}
	if got, want := estimateNodeEnergyCost(unknown, &task.Task{}), estimateNodeEnergyCost(atDefault, &task.Task{}); got != want {
		t.Fatalf("expected unknown carbon to cost like the default factor (%v), got %v", want, got)
	}
}

// The point of the fixed scale: a spread between uniformly clean grids must
// barely move the score, while a real spread must.
func TestEnergyCostSpreadDependsOnAbsoluteIntensityNotJustRank(t *testing.T) {
	spread := func(intensities ...float64) float64 {
		lo, hi := 1e9, -1e9
		for _, v := range intensities {
			c := estimateNodeEnergyCost(&node.Node{CarbonIntensity: node.CarbonOf(v)}, &task.Task{})
			if c < lo {
				lo = c
			}
			if c > hi {
				hi = c
			}
		}
		return hi - lo
	}

	clean := spread(0, 0.004, 0.015) // 0, 4, 15 gCO2/kWh
	real := spread(0, 0.100, 0.237)  // 0, 100, 237 gCO2/kWh
	if clean >= 5 {
		t.Fatalf("expected a spread between very clean grids to stay small, got %v", clean)
	}
	if real <= 5*clean {
		t.Fatalf("expected a real carbon spread to matter far more: clean=%v real=%v", clean, real)
	}
}

// Carbon at or above the reference saturates, so two very dirty grids cost the
// same and the scheduler does not chase differences that no longer matter.
func TestEnergyCostSaturatesAtReferenceIntensity(t *testing.T) {
	a := estimateNodeEnergyCost(&node.Node{CarbonIntensity: node.CarbonOf(refCarbonFactor)}, &task.Task{})
	b := estimateNodeEnergyCost(&node.Node{CarbonIntensity: node.CarbonOf(refCarbonFactor * 3)}, &task.Task{})
	if a != b {
		t.Fatalf("expected saturation at the reference intensity, got %v vs %v", a, b)
	}
}
