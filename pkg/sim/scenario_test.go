package sim

import "testing"

func TestDefaultScenarioIsValid(t *testing.T) {
	if err := DefaultScenario().Validate(); err != nil {
		t.Fatalf("built-in scenario invalid: %v", err)
	}
}

// The built-in scenario must contain the network-vs-carbon tension the
// benchmark is meant to study: the cleanest zone is the farthest from the
// dirtiest one.
func TestDefaultScenarioHasNetworkCarbonTension(t *testing.T) {
	s := DefaultScenario()
	clean, _ := s.Node("scot-1")
	dirty, _ := s.Node("south-1")
	mid, _ := s.Node("nw-1")

	if !(clean.CarbonGPerKWh < mid.CarbonGPerKWh && mid.CarbonGPerKWh < dirty.CarbonGPerKWh) {
		t.Fatalf("expected scot < nw < south carbon, got %v %v %v", clean.CarbonGPerKWh, mid.CarbonGPerKWh, dirty.CarbonGPerKWh)
	}
	far, _ := s.Topology.GetLatency(clean.Name, dirty.Name)
	near, _ := s.Topology.GetLatency(mid.Name, dirty.Name)
	if far <= near {
		t.Fatalf("expected cleanest zone to be farthest from the dirtiest, got far=%v near=%v", far, near)
	}
}

func TestValidateRejectsAsymmetricLatency(t *testing.T) {
	s := DefaultScenario()
	s.Topology.NodeLatencies["scot-1"]["south-1"] = 999
	if err := s.Validate(); err == nil {
		t.Fatal("expected asymmetric latency to be rejected")
	}
}

func TestValidateRejectsMissingLatency(t *testing.T) {
	s := DefaultScenario()
	delete(s.Topology.NodeLatencies["nw-1"], "nw-2")
	if err := s.Validate(); err == nil {
		t.Fatal("expected missing latency to be rejected")
	}
}

func TestCarbonFactorConvertsToSchedulerUnits(t *testing.T) {
	if got := (NodeSpec{CarbonGPerKWh: 350}).CarbonFactor(); got != 0.35 {
		t.Fatalf("expected 0.35, got %v", got)
	}
}
