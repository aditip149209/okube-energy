package sim

import (
	"reflect"
	"testing"
)

// Behavioural sanity checks on the whole harness. A single trace is too noisy
// to assert on (60 deployments), so properties that should hold in general are
// checked on averages over several seeds.

const sanitySeeds = 10

type seedAverages struct {
	dynamicCO2G   float64
	meanLatencyMS float64
	flexibleInt   float64 // mean grid intensity of nodes used by flexible tasks
	criticalInt   float64
}

func averageOverSeeds(t *testing.T, cfg Config) seedAverages {
	t.Helper()
	s := DefaultScenario()
	var avg seedAverages
	for seed := int64(1); seed <= sanitySeeds; seed++ {
		p := DefaultTraceParams()
		p.Seed = seed
		trace, err := GenerateTrace(s, p)
		if err != nil {
			t.Fatal(err)
		}
		res, err := Run(s, trace, cfg, RunOptions{TieBreakSeed: seed})
		if err != nil {
			t.Fatalf("%s seed %d: %v", cfg.Name, seed, err)
		}
		m := Summarize(s, res)
		avg.dynamicCO2G += m.DynamicCO2G / sanitySeeds
		avg.meanLatencyMS += m.MeanLatencyMS / sanitySeeds
		avg.flexibleInt += m.ByClass[ClassFlexible].MeanIntensityGKWh / sanitySeeds
		avg.criticalInt += m.ByClass[ClassCritical].MeanIntensityGKWh / sanitySeeds
	}
	return avg
}

func configNamed(name string) Config {
	for _, c := range Configs() {
		if c.Name == name {
			return c
		}
	}
	panic("no config " + name)
}

// The true-baseline guarantee: a fused run with energy weight 0 must make
// exactly the same placements as network-only.
func TestFusedWithZeroEnergyWeightEqualsNetworkOnly(t *testing.T) {
	s := DefaultScenario()
	trace := defaultTrace(t, s)

	fusedZero := configNamed(ConfigFused).WithEnergyWeight(0)
	a := nodesOf(mustRun(t, s, trace, fusedZero))
	b := nodesOf(mustRun(t, s, trace, configNamed(ConfigNetworkOnly)))
	if !reflect.DeepEqual(a, b) {
		t.Fatal("fused with w_energy=0 diverged from network-only")
	}
}

func TestNetworkAwarenessCutsLatency(t *testing.T) {
	def := averageOverSeeds(t, configNamed(ConfigDefault))
	net := averageOverSeeds(t, configNamed(ConfigNetworkOnly))
	if net.meanLatencyMS >= def.meanLatencyMS/2 {
		t.Fatalf("expected network-only latency well below default, got %.2f vs %.2f ms", net.meanLatencyMS, def.meanLatencyMS)
	}
}

func TestEnergyAwarenessMovesFlexibleWorkToCleanerNodes(t *testing.T) {
	def := averageOverSeeds(t, configNamed(ConfigDefault))
	energy := averageOverSeeds(t, configNamed(ConfigEnergyOnly).WithEnergyWeight(2.0))

	if energy.flexibleInt >= def.flexibleInt {
		t.Fatalf("expected flexible tasks on cleaner nodes, got %.1f vs default %.1f g/kWh", energy.flexibleInt, def.flexibleInt)
	}
	if energy.dynamicCO2G >= def.dynamicCO2G {
		t.Fatalf("expected lower CO2 than default, got %.2f vs %.2f g", energy.dynamicCO2G, def.dynamicCO2G)
	}
	// The x1.4 / x0.35 class weights: flexible work must end up on cleaner
	// nodes than critical work.
	if energy.flexibleInt >= energy.criticalInt {
		t.Fatalf("expected flexible on cleaner nodes than critical, got %.1f vs %.1f g/kWh", energy.flexibleInt, energy.criticalInt)
	}
}

// The headline comparison: fusing keeps network-only's latency while cutting
// carbon.
func TestFusedKeepsLatencyAndCutsCarbonVersusNetworkOnly(t *testing.T) {
	net := averageOverSeeds(t, configNamed(ConfigNetworkOnly))
	fused := averageOverSeeds(t, configNamed(ConfigFused))
	def := averageOverSeeds(t, configNamed(ConfigDefault))

	if fused.dynamicCO2G >= net.dynamicCO2G {
		t.Errorf("expected fused CO2 below network-only, got %.2f vs %.2f g", fused.dynamicCO2G, net.dynamicCO2G)
	}
	if fused.meanLatencyMS >= def.meanLatencyMS/2 {
		t.Errorf("expected fused latency to stay close to network-only, got %.2f (default %.2f, network-only %.2f ms)",
			fused.meanLatencyMS, def.meanLatencyMS, net.meanLatencyMS)
	}
}
