package sim

import (
	"math"
	"testing"
)

func smallOptions() ExperimentOptions {
	return ExperimentOptions{
		Seeds:        3,
		Workloads:    []Workload{{Name: "default", Params: DefaultTraceParams()}},
		SweepWeights: []float64{0, 1},
	}
}

func findAgg(t *testing.T, agg []AggRow, workload, config, weight string) AggRow {
	t.Helper()
	for _, a := range agg {
		if a.Workload == workload && a.Config == config && a.EnergyWeight == weight {
			return a
		}
	}
	t.Fatalf("no aggregate row for %s/%s/%s", workload, config, weight)
	return AggRow{}
}

func TestNewStat(t *testing.T) {
	s := newStat([]float64{2, 4, 4, 4, 5, 5, 7, 9})
	if !approx(s.Mean, 5) || math.Abs(s.Std-2.13808993529939) > 1e-9 {
		t.Fatalf("mean/std = %v / %v, want 5 / 2.138...", s.Mean, s.Std)
	}
	if one := newStat([]float64{3}); one.Mean != 3 || one.Std != 0 {
		t.Fatalf("single value stat = %+v", one)
	}
	if (newStat(nil) != Stat{}) {
		t.Fatal("empty input should give zero stat")
	}
}

func TestRunExperimentRowCounts(t *testing.T) {
	opts := smallOptions()
	rows, err := RunExperiment(DefaultScenario(), opts)
	if err != nil {
		t.Fatal(err)
	}
	// Per seed: 4 default-weight configs + 2 swept configs x 2 weights.
	if want := opts.Seeds * (4 + 2*len(opts.SweepWeights)); len(rows) != want {
		t.Fatalf("got %d rows, want %d", len(rows), want)
	}
}

func TestRunExperimentRejectsBadOptions(t *testing.T) {
	if _, err := RunExperiment(DefaultScenario(), ExperimentOptions{}); err == nil {
		t.Fatal("expected empty options to be rejected")
	}
}

func TestAggregateIsPairedAgainstDefault(t *testing.T) {
	rows, _ := RunExperiment(DefaultScenario(), smallOptions())
	agg := Aggregate(rows)

	def := findAgg(t, agg, "default", ConfigDefault, WeightDefault)
	if def.Runs != 3 || def.CO2VsDefaultPct.Mean != 0 || def.LatencyVsDefaultPct.Mean != 0 {
		t.Fatalf("default config must be 0%% vs itself over 3 runs, got %+v", def)
	}

	net := findAgg(t, agg, "default", ConfigNetworkOnly, WeightDefault)
	if net.LatencyVsDefaultPct.Mean >= -50 {
		t.Fatalf("expected network-only latency well below default, got %.1f%%", net.LatencyVsDefaultPct.Mean)
	}
}

// The sweep's w=0 point is the true network-only baseline.
func TestSweepZeroEqualsNetworkOnly(t *testing.T) {
	rows, _ := RunExperiment(DefaultScenario(), smallOptions())
	agg := Aggregate(rows)

	net := findAgg(t, agg, "default", ConfigNetworkOnly, WeightDefault)
	zero := findAgg(t, agg, "default", ConfigFused, "0")
	if net.DynamicCO2G != zero.DynamicCO2G || net.MeanLatencyMS != zero.MeanLatencyMS {
		t.Fatalf("fused w=0 diverged from network-only: %+v vs %+v", zero.DynamicCO2G, net.DynamicCO2G)
	}
}

func TestExperimentIsReproducible(t *testing.T) {
	a := Aggregate(mustExperiment(t))
	b := Aggregate(mustExperiment(t))
	for i := range a {
		// Decision time is wall-clock, everything else must match exactly.
		a[i].MeanDecisionUS, b[i].MeanDecisionUS = Stat{}, Stat{}
		if a[i] != b[i] {
			t.Fatalf("row %d differs between identical experiments:\n%+v\n%+v", i, a[i], b[i])
		}
	}
}

func mustExperiment(t *testing.T) []RunRow {
	t.Helper()
	rows, err := RunExperiment(DefaultScenario(), smallOptions())
	if err != nil {
		t.Fatal(err)
	}
	return rows
}
