package sim

import (
	"testing"

	"github.com/aditip149209/okube/pkg/scheduler"
)

func TestConfigsMatchEvaluationMatrix(t *testing.T) {
	want := map[string][2]bool{ // network on, energy on
		ConfigDefault:     {false, false},
		ConfigNetworkOnly: {true, false},
		ConfigEnergyOnly:  {false, true},
		ConfigFused:       {true, true},
	}
	cfgs := Configs()
	if len(cfgs) != len(want) {
		t.Fatalf("expected %d configs, got %d", len(want), len(cfgs))
	}
	for _, c := range cfgs {
		w, ok := want[c.Name]
		if !ok {
			t.Fatalf("unexpected config %q", c.Name)
		}
		sc := c.ScoreContext(nil)
		if got := [2]bool{!sc.DisableNetworkScore, !sc.DisableEnergyScore}; got != w {
			t.Errorf("%s: network/energy on = %v, want %v", c.Name, got, w)
		}
	}
}

func TestNetworkBlindConfigsDropFilterContext(t *testing.T) {
	filter := &scheduler.FilterContext{}
	for _, c := range Configs() {
		got := c.FilterContextFor(filter)
		if c.NetworkAware() && got != filter {
			t.Errorf("%s: network-aware config should keep the filter context", c.Name)
		}
		if !c.NetworkAware() && got != nil {
			t.Errorf("%s: network-blind config must not see the filter context", c.Name)
		}
		if !c.NetworkAware() && c.ScoreContext(filter).Filter != nil {
			t.Errorf("%s: network-blind ScoreContext must not carry the filter context", c.Name)
		}
	}
}

func TestWithEnergyWeightHonoursExplicitZero(t *testing.T) {
	base := Configs()[3]
	zero := base.WithEnergyWeight(0)
	if zero.EnergyWeight == nil || *zero.EnergyWeight != 0 {
		t.Fatalf("expected explicit zero weight, got %v", zero.EnergyWeight)
	}
	if base.EnergyWeight != nil {
		t.Fatal("WithEnergyWeight must not mutate the original config")
	}
}
