package sim

import (
	"math"
	"testing"
	"time"

	"github.com/aditip149209/okube/pkg/power"
)

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestPercentileNearestRank(t *testing.T) {
	vals := []float64{15, 20, 35, 40, 50}
	cases := map[float64]float64{0.05: 15, 0.3: 20, 0.4: 20, 0.5: 35, 0.95: 50, 1: 50}
	for p, want := range cases {
		if got := percentile(vals, p); got != want {
			t.Errorf("p%v: got %v, want %v", p, got, want)
		}
	}
	if percentile(nil, 0.95) != 0 {
		t.Error("empty input should give 0")
	}
	if vals[0] != 15 {
		t.Error("percentile must not reorder its input")
	}
}

// One 1-core task for an hour on an 8-core node (100-250 W) at 100 g/kWh:
// dynamic energy = 150 W * (1/8) * 1 h = 0.01875 kWh, i.e. 1.875 g CO2.
func TestSummarizeHandComputedCO2(t *testing.T) {
	s := &Scenario{Nodes: []NodeSpec{{
		Name: "n", Cores: 8, MemoryKB: 1 << 20, CarbonGPerKWh: 100, Power: power.Linear(100, 250),
	}}}
	res := &Result{Config: "x", MakespanSec: 3600, Placements: []Placement{{
		Task: TraceTask{ID: "t", CPU: 1, DurationSec: 3600, DeadlineSec: 5000, Class: ClassDefault},
		Node: "n", StartSec: 0, EndSec: 3600,
	}}}

	sum := Summarize(s, res)
	if !approx(sum.DynamicEnergyKWh, 0.01875) || !approx(sum.DynamicCO2G, 1.875) {
		t.Fatalf("dynamic energy/CO2 = %v / %v, want 0.01875 / 1.875", sum.DynamicEnergyKWh, sum.DynamicCO2G)
	}
	// Idle: 100 W for 1 h = 0.1 kWh -> 10 g.
	if !approx(sum.IdleEnergyKWh, 0.1) || !approx(sum.IdleCO2G, 10) || !approx(sum.TotalCO2G, 11.875) {
		t.Fatalf("idle energy/CO2/total = %v / %v / %v", sum.IdleEnergyKWh, sum.IdleCO2G, sum.TotalCO2G)
	}
}

func TestSummarizeDeadlinesLatencyAndClasses(t *testing.T) {
	s := &Scenario{Nodes: []NodeSpec{{
		Name: "n", Cores: 4, MemoryKB: 1 << 20, CarbonGPerKWh: 200, Power: power.Linear(100, 200),
	}}}
	mk := func(id string, class Class, end, deadline, lat float64, hasDeps bool, wait float64) Placement {
		return Placement{
			Task: TraceTask{ID: id, CPU: 1, DurationSec: 10, DeadlineSec: deadline, Class: class},
			Node: "n", EndSec: end, QueueWaitSec: wait,
			HasDependencies: hasDeps, DependencyLatencyMS: lat, Decision: 10 * time.Microsecond,
		}
	}
	res := &Result{Config: "x", MakespanSec: 100, Placements: []Placement{
		mk("a", ClassCritical, 50, 40, 10, true, 0),  // late
		mk("b", ClassFlexible, 50, 60, 30, true, 4),  // on time
		mk("c", ClassFlexible, 50, 60, 0, false, 0),  // root: excluded from latency
		mk("d", ClassDefault, 50, 49.9, 20, true, 0), // late
	}}

	sum := Summarize(s, res)
	if sum.DeadlineViolations != 2 || !approx(sum.DeadlineViolationRate, 0.5) {
		t.Errorf("violations = %d (%v), want 2 (0.5)", sum.DeadlineViolations, sum.DeadlineViolationRate)
	}
	if sum.LatencyTasks != 3 || !approx(sum.MeanLatencyMS, 20) || !approx(sum.P95LatencyMS, 30) {
		t.Errorf("latency tasks/mean/p95 = %d / %v / %v, want 3 / 20 / 30", sum.LatencyTasks, sum.MeanLatencyMS, sum.P95LatencyMS)
	}
	if !approx(sum.MeanQueueWaitSec, 1) {
		t.Errorf("mean queue wait = %v, want 1", sum.MeanQueueWaitSec)
	}
	if got := sum.ByClass[ClassFlexible]; got.Tasks != 2 || got.DeadlineViolations != 0 || !approx(got.MeanIntensityGKWh, 200) {
		t.Errorf("flexible class stats = %+v", got)
	}
	if got := sum.ByClass[ClassCritical]; got.DeadlineViolations != 1 {
		t.Errorf("critical class stats = %+v", got)
	}
	if !approx(sum.MeanDecisionUS, 10) {
		t.Errorf("mean decision = %vus, want 10", sum.MeanDecisionUS)
	}
}

// With a non-linear curve and overlapping tasks, dynamic power depends on total
// utilisation, not on each task alone. 4-core node, curve 100 W idle,
// 200 W at 50%, 400 W at 100%; carbon 360 g/kWh (so 1 kWh = 360 g).
//
//	A: 1 core, t=0..100     B: 1 core, t=50..150
//	[0,50)   u=0.25 -> 150 W, dynamic  50 W, A alone
//	[50,100) u=0.50 -> 200 W, dynamic 100 W, A and B share 50 W each
//	[100,150) u=0.25 -> 150 W, dynamic  50 W, B alone
//
// A and B each get 50*50 + 50*50 = 5000 W*s; the node total is 10000 W*s.
func TestSummarizeIntegratesNonLinearCurveOverOverlap(t *testing.T) {
	s := &Scenario{Nodes: []NodeSpec{{
		Name: "n", Cores: 4, MemoryKB: 1 << 20, CarbonGPerKWh: 360,
		Power: &power.Curve{Utilisation: []float64{0, 0.5, 1}, Watts: []float64{100, 200, 400}},
	}}}
	task := func(id string, class Class) TraceTask {
		return TraceTask{ID: id, CPU: 1, DurationSec: 100, DeadlineSec: 1e9, Class: class}
	}
	res := &Result{Config: "x", MakespanSec: 150, Placements: []Placement{
		{Task: task("a", ClassCritical), Node: "n", StartSec: 0, EndSec: 100},
		{Task: task("b", ClassFlexible), Node: "n", StartSec: 50, EndSec: 150},
	}}

	sum := Summarize(s, res)
	wantKWh := 10000.0 / joulesPerKWh
	if !approx(sum.DynamicEnergyKWh, wantKWh) {
		t.Fatalf("dynamic energy = %v kWh, want %v", sum.DynamicEnergyKWh, wantKWh)
	}
	if !approx(sum.DynamicCO2G, wantKWh*360) {
		t.Fatalf("dynamic CO2 = %v g, want %v", sum.DynamicCO2G, wantKWh*360)
	}
	for _, c := range []Class{ClassCritical, ClassFlexible} {
		if got, want := sum.ByClass[c].DynamicCO2G, 5000.0/joulesPerKWh*360; !approx(got, want) {
			t.Errorf("%s CO2 = %v g, want %v", c, got, want)
		}
	}
	// Idle: 100 W for 150 s.
	if !approx(sum.IdleEnergyKWh, 100.0*150/joulesPerKWh) {
		t.Errorf("idle energy = %v kWh", sum.IdleEnergyKWh)
	}
}

// Two tasks that never overlap must cost the same as the old per-task formula
// on a linear curve: (max-idle) * cpu/cores * duration.
func TestSummarizeLinearCurveMatchesPerTaskFormula(t *testing.T) {
	s := &Scenario{Nodes: []NodeSpec{{
		Name: "n", Cores: 8, MemoryKB: 1 << 20, CarbonGPerKWh: 100, Power: power.Linear(100, 250),
	}}}
	res := &Result{Config: "x", MakespanSec: 7200, Placements: []Placement{
		{Task: TraceTask{ID: "a", CPU: 2, DurationSec: 3600, DeadlineSec: 1e9}, Node: "n", StartSec: 0, EndSec: 3600},
		{Task: TraceTask{ID: "b", CPU: 1, DurationSec: 3600, DeadlineSec: 1e9}, Node: "n", StartSec: 3600, EndSec: 7200},
	}}
	// 150 W * (2/8) * 1 h + 150 W * (1/8) * 1 h = 0.0375 + 0.01875 kWh.
	if got, want := Summarize(s, res).DynamicEnergyKWh, 0.05625; !approx(got, want) {
		t.Fatalf("dynamic energy = %v kWh, want %v", got, want)
	}
}
