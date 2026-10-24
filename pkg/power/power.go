// Package power models how many watts a node draws at a given CPU
// utilisation. Curves are produced by models/train_power_model.py from
// SPECpower_ssj2008 results and loaded from JSON; the simulator uses them to
// turn utilisation over time into energy.
package power

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

// defaultCurves is a copy of models/artifacts/power_curves.json. After
// retraining, refresh it with:
//
//	cp models/artifacts/power_curves.json pkg/power/default_curves.json
//
//go:embed default_curves.json
var defaultCurves []byte

// Curve is a piecewise-linear power curve: Watts[i] is the draw at CPU
// utilisation Utilisation[i], a fraction in [0,1].
type Curve struct {
	Utilisation []float64
	Watts       []float64
}

// Linear returns the two-point curve from idle watts at 0% to max watts at
// 100% utilisation.
func Linear(idleW, maxW float64) *Curve {
	return &Curve{Utilisation: []float64{0, 1}, Watts: []float64{idleW, maxW}}
}

// At returns the power draw at utilisation u, interpolating between points.
// u is clamped to [0,1], so over-committed CPU cannot extrapolate past the
// measured 100% point.
func (c *Curve) At(u float64) float64 {
	if u <= c.Utilisation[0] {
		return c.Watts[0]
	}
	last := len(c.Utilisation) - 1
	if u >= c.Utilisation[last] {
		return c.Watts[last]
	}
	i := sort.SearchFloat64s(c.Utilisation, u) // first index with Utilisation[i] >= u
	if c.Utilisation[i] == u {
		return c.Watts[i]
	}
	x0, x1 := c.Utilisation[i-1], c.Utilisation[i]
	y0, y1 := c.Watts[i-1], c.Watts[i]
	return y0 + (y1-y0)*(u-x0)/(x1-x0)
}

// Idle is the draw at 0% utilisation.
func (c *Curve) Idle() float64 { return c.Watts[0] }

// Validate checks the curve is usable: matching lengths, utilisation strictly
// increasing from 0 to 1, and positive watts that never fall as load rises.
func (c *Curve) Validate() error {
	if c == nil {
		return fmt.Errorf("nil curve")
	}
	n := len(c.Utilisation)
	if n < 2 || len(c.Watts) != n {
		return fmt.Errorf("need at least 2 points and equal lengths, got %d utilisation and %d watts", n, len(c.Watts))
	}
	if c.Utilisation[0] != 0 || c.Utilisation[n-1] != 1 {
		return fmt.Errorf("utilisation must span 0 to 1, got %v to %v", c.Utilisation[0], c.Utilisation[n-1])
	}
	for i := 0; i < n; i++ {
		if c.Watts[i] <= 0 {
			return fmt.Errorf("watts must be positive, got %v at point %d", c.Watts[i], i)
		}
		if i > 0 {
			if c.Utilisation[i] <= c.Utilisation[i-1] {
				return fmt.Errorf("utilisation must be strictly increasing at point %d", i)
			}
			if c.Watts[i] < c.Watts[i-1] {
				return fmt.Errorf("watts must not fall as utilisation rises, point %d", i)
			}
		}
	}
	return nil
}

type file struct {
	NodeTypes map[string]struct {
		Utilisation []float64 `json:"utilisation"`
		Watts       []float64 `json:"watts"`
	} `json:"node_types"`
}

// Parse reads the power_curves.json format and validates every curve.
func Parse(data []byte) (map[string]*Curve, error) {
	var f file
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("power: parse curves: %w", err)
	}
	if len(f.NodeTypes) == 0 {
		return nil, fmt.Errorf("power: no node_types in curves file")
	}
	out := make(map[string]*Curve, len(f.NodeTypes))
	for name, nt := range f.NodeTypes {
		c := &Curve{Utilisation: nt.Utilisation, Watts: nt.Watts}
		if err := c.Validate(); err != nil {
			return nil, fmt.Errorf("power: node type %q: %w", name, err)
		}
		out[name] = c
	}
	return out, nil
}

// Load reads curves from a power_curves.json file.
func Load(path string) (map[string]*Curve, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("power: read %q: %w", path, err)
	}
	return Parse(data)
}

// Default returns the curves embedded in the binary.
func Default() (map[string]*Curve, error) {
	return Parse(defaultCurves)
}
