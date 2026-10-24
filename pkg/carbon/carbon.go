// Package carbon loads per-node carbon intensity values from a static JSON
// config file. It stands in for a live grid carbon intensity API (e.g.
// Electricity Maps, WattTime) during evaluation, without coupling the
// scheduler to a specific provider.
package carbon

import (
	"encoding/json"
	"fmt"
	"os"
)

// Intensity holds a default carbon intensity plus optional per-node
// overrides, keyed by worker ID or worker address (matching whichever the
// operator finds easier to author by hand).
type Intensity struct {
	Default float64            `json:"default"`
	Nodes   map[string]float64 `json:"nodes"`
}

// Load reads a carbon intensity config file from path.
func Load(path string) (*Intensity, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("carbon: read config %q: %w", path, err)
	}

	var i Intensity
	if err := json.Unmarshal(data, &i); err != nil {
		return nil, fmt.Errorf("carbon: parse config %q: %w", path, err)
	}
	if i.Nodes == nil {
		i.Nodes = map[string]float64{}
	}

	return &i, nil
}

// Lookup returns the carbon intensity for a node, checked first by ID then by
// address, falling back to the configured default. The bool is false only for
// a nil receiver (no config loaded), so callers can tell "no data" apart from
// a genuine intensity of 0 (a fully low-carbon grid).
func (i *Intensity) Lookup(id, address string) (float64, bool) {
	if i == nil {
		return 0, false
	}
	if v, ok := i.Nodes[id]; ok {
		return v, true
	}
	if v, ok := i.Nodes[address]; ok {
		return v, true
	}
	return i.Default, true
}

// For is Lookup without the "known" flag: a nil receiver returns 0. Prefer
// Lookup where the difference between "unknown" and 0 matters.
func (i *Intensity) For(id, address string) float64 {
	v, _ := i.Lookup(id, address)
	return v
}
