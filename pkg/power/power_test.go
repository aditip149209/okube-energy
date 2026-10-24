package power

import (
	"bytes"
	"math"
	"os"
	"testing"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestAtInterpolatesAndClamps(t *testing.T) {
	c := &Curve{Utilisation: []float64{0, 0.5, 1}, Watts: []float64{100, 200, 400}}
	cases := map[float64]float64{
		0: 100, 0.25: 150, 0.5: 200, 0.75: 300, 1: 400,
		-0.3: 100, // below range clamps to idle
		1.7:  400, // over-committed CPU does not extrapolate
	}
	for u, want := range cases {
		if got := c.At(u); !near(got, want) {
			t.Errorf("At(%v) = %v, want %v", u, got, want)
		}
	}
}

func TestLinearMatchesIdleToMaxLine(t *testing.T) {
	c := Linear(100, 250)
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	if got := c.At(0.5); !near(got, 175) {
		t.Fatalf("At(0.5) = %v, want 175", got)
	}
	if c.Idle() != 100 {
		t.Fatalf("Idle() = %v, want 100", c.Idle())
	}
}

func TestValidateRejectsBadCurves(t *testing.T) {
	bad := map[string]*Curve{
		"nil":              nil,
		"one point":        {Utilisation: []float64{0}, Watts: []float64{10}},
		"length mismatch":  {Utilisation: []float64{0, 1}, Watts: []float64{10}},
		"does not reach 1": {Utilisation: []float64{0, 0.9}, Watts: []float64{10, 20}},
		"not increasing":   {Utilisation: []float64{0, 0.5, 0.5, 1}, Watts: []float64{10, 20, 30, 40}},
		"power falls":      {Utilisation: []float64{0, 0.5, 1}, Watts: []float64{10, 30, 20}},
		"non-positive":     {Utilisation: []float64{0, 1}, Watts: []float64{0, 20}},
	}
	for name, c := range bad {
		if err := c.Validate(); err == nil {
			t.Errorf("%s: expected a validation error", name)
		}
	}
}

func TestDefaultCurvesAreValidAndPhysical(t *testing.T) {
	curves, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	small, ok := curves["small-8c"]
	if !ok {
		t.Fatal("embedded curves are missing node type small-8c")
	}
	// A server draws more at full load than idle, and idle is a real
	// fraction of peak, not ~0 or ~peak.
	if ratio := small.Idle() / small.At(1); ratio < 0.1 || ratio > 0.7 {
		t.Fatalf("implausible idle/peak ratio %.2f", ratio)
	}
}

func TestParseRejectsEmptyAndBadJSON(t *testing.T) {
	if _, err := Parse([]byte(`{}`)); err == nil {
		t.Error("expected an error for a file with no node types")
	}
	if _, err := Parse([]byte(`not json`)); err == nil {
		t.Error("expected an error for invalid JSON")
	}
}

// Catches retraining without refreshing the embedded copy.
func TestEmbeddedCurvesMatchTrainingOutput(t *testing.T) {
	trained, err := os.ReadFile("../../models/artifacts/power_curves.json")
	if err != nil {
		t.Skip("no training output present")
	}
	if !bytes.Equal(trained, defaultCurves) {
		t.Fatal("pkg/power/default_curves.json differs from models/artifacts/power_curves.json; " +
			"run: cp models/artifacts/power_curves.json pkg/power/default_curves.json")
	}
}
