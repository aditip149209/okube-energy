// Package sim is a deterministic simulator that drives the real scheduler
// pipeline over a synthetic cluster and workload trace, so the default,
// network-only, energy-only and fused configurations can be compared on
// identical inputs.
package sim

import (
	"fmt"
	"math"

	"github.com/aditip149209/okube/pkg/appgroup"
	"github.com/aditip149209/okube/pkg/topology"
)

// NodeSpec describes one simulated worker.
type NodeSpec struct {
	Name     string
	Zone     string
	Cores    int
	MemoryKB int64 // KB, matching what a worker reports from /proc/meminfo

	// CarbonGPerKWh is the grid carbon intensity in gCO2/kWh. It is kept in
	// real units so a live feed can replace it later; the scheduler's unitless
	// factor is derived via CarbonFactor.
	CarbonGPerKWh float64

	// Linear power model: power(u) = IdleWatts + (MaxWatts-IdleWatts)*u for
	// CPU utilisation u in [0,1].
	IdleWatts float64
	MaxWatts  float64
}

// CarbonFactor converts gCO2/kWh into the unitless per-node factor the
// scheduler's energy stage consumes (e.g. 350 g/kWh -> 0.35).
func (n NodeSpec) CarbonFactor() float64 {
	return n.CarbonGPerKWh / 1000.0
}

// Scenario is the simulated cluster: nodes, the network between them, and the
// application dependency graphs that workloads are instances of.
type Scenario struct {
	Nodes     []NodeSpec
	Topology  *topology.NetworkTopology
	AppGroups []*appgroup.AppGroup
}

// Node returns the spec with the given name.
func (s *Scenario) Node(name string) (NodeSpec, bool) {
	for _, n := range s.Nodes {
		if n.Name == name {
			return n, true
		}
	}
	return NodeSpec{}, false
}

// AppGroup returns the application template with the given ID.
func (s *Scenario) AppGroup(appID string) (*appgroup.AppGroup, bool) {
	for _, g := range s.AppGroups {
		if g.AppID == appID {
			return g, true
		}
	}
	return nil, false
}

// Validate checks that the scenario is internally consistent: unique,
// well-formed nodes, a complete symmetric latency matrix with a zero
// diagonal, and uniquely named app groups.
func (s *Scenario) Validate() error {
	if len(s.Nodes) == 0 {
		return fmt.Errorf("scenario has no nodes")
	}
	if len(s.AppGroups) == 0 {
		return fmt.Errorf("scenario has no app groups")
	}

	seen := make(map[string]struct{}, len(s.Nodes))
	for _, n := range s.Nodes {
		if n.Name == "" {
			return fmt.Errorf("node with empty name")
		}
		if _, dup := seen[n.Name]; dup {
			return fmt.Errorf("duplicate node %q", n.Name)
		}
		seen[n.Name] = struct{}{}
		if n.Cores <= 0 || n.MemoryKB <= 0 {
			return fmt.Errorf("node %q needs positive cores and memory", n.Name)
		}
		if n.CarbonGPerKWh < 0 {
			return fmt.Errorf("node %q has a negative carbon intensity", n.Name)
		}
		if n.IdleWatts <= 0 || n.MaxWatts < n.IdleWatts {
			return fmt.Errorf("node %q needs 0 < idle watts <= max watts", n.Name)
		}
	}

	if s.Topology == nil {
		return fmt.Errorf("scenario has no topology")
	}
	for _, a := range s.Nodes {
		for _, b := range s.Nodes {
			ab, ok := s.Topology.GetLatency(a.Name, b.Name)
			if !ok {
				return fmt.Errorf("missing latency %s -> %s", a.Name, b.Name)
			}
			ba, ok := s.Topology.GetLatency(b.Name, a.Name)
			if !ok {
				return fmt.Errorf("missing latency %s -> %s", b.Name, a.Name)
			}
			if math.Abs(ab-ba) > 1e-9 {
				return fmt.Errorf("asymmetric latency %s<->%s: %v vs %v", a.Name, b.Name, ab, ba)
			}
			if a.Name == b.Name && ab != 0 {
				return fmt.Errorf("non-zero self latency on %s", a.Name)
			}
		}
	}

	groups := make(map[string]struct{}, len(s.AppGroups))
	for _, g := range s.AppGroups {
		if _, dup := groups[g.AppID]; dup {
			return fmt.Errorf("duplicate app group %q", g.AppID)
		}
		groups[g.AppID] = struct{}{}
	}
	return nil
}

// DefaultScenario returns the built-in scenario: six identical-capacity nodes
// in three zones, with a deliberate tension between network and carbon. The
// cleanest zone (scot) is the farthest from the dirtiest (south), so a
// latency-driven placement and a carbon-driven placement disagree.
//
// Carbon intensities are illustrative UK-like regional values in gCO2/kWh, not
// measured data.
func DefaultScenario() *Scenario {
	const (
		cores    = 8
		memoryKB = 32 * 1024 * 1024 // 32 GiB
		idleW    = 100.0
		maxW     = 250.0
	)

	zones := []struct {
		name   string
		carbon float64
	}{
		{"scot", 50},
		{"nw", 180},
		{"south", 260},
	}

	var nodes []NodeSpec
	zoneOf := make(map[string]string)
	for _, z := range zones {
		for i := 1; i <= 2; i++ {
			name := fmt.Sprintf("%s-%d", z.name, i)
			nodes = append(nodes, NodeSpec{
				Name: name, Zone: z.name, Cores: cores, MemoryKB: memoryKB,
				CarbonGPerKWh: z.carbon, IdleWatts: idleW, MaxWatts: maxW,
			})
			zoneOf[name] = z.name
		}
	}

	// Latency (ms) and bandwidth (Mbps) by zone pair; same node is 0 ms.
	zoneLatency := map[[2]string]float64{
		{"scot", "scot"}: 2, {"nw", "nw"}: 2, {"south", "south"}: 2,
		{"scot", "nw"}: 12, {"nw", "south"}: 10, {"scot", "south"}: 22,
	}
	lookup := func(a, b string) float64 {
		if v, ok := zoneLatency[[2]string{a, b}]; ok {
			return v
		}
		return zoneLatency[[2]string{b, a}]
	}

	topo := &topology.NetworkTopology{
		NodeLatencies: map[string]map[string]float64{},
		NodeBandwidth: map[string]map[string]float64{},
		ZoneMapping:   zoneOf,
	}
	for _, a := range nodes {
		topo.NodeLatencies[a.Name] = map[string]float64{}
		topo.NodeBandwidth[a.Name] = map[string]float64{}
		for _, b := range nodes {
			lat, bw := 0.0, 10000.0
			if a.Name != b.Name {
				lat = lookup(a.Zone, b.Zone)
				bw = 1000
				if a.Zone != b.Zone {
					bw = 400
				}
			}
			topo.NodeLatencies[a.Name][b.Name] = lat
			topo.NodeBandwidth[a.Name][b.Name] = bw
		}
	}

	// Edges read "From depends on To".
	mustGroup := func(id string, services []string, edges ...[2]string) *appgroup.AppGroup {
		var es []appgroup.DependencyEdge
		for _, e := range edges {
			es = append(es, appgroup.DependencyEdge{From: e[0], To: e[1]})
		}
		g, err := appgroup.NewAppGroup(id, services, es)
		if err != nil {
			panic(fmt.Sprintf("sim: invalid built-in app group %q: %v", id, err))
		}
		return g
	}

	return &Scenario{
		Nodes:    nodes,
		Topology: topo,
		AppGroups: []*appgroup.AppGroup{
			mustGroup("shop", []string{"web", "api", "db", "cache"},
				[2]string{"web", "api"}, [2]string{"api", "db"}, [2]string{"api", "cache"}),
			mustGroup("analytics", []string{"dash", "etl", "warehouse"},
				[2]string{"dash", "warehouse"}, [2]string{"etl", "warehouse"}),
			mustGroup("media", []string{"gateway", "transcode", "blobstore"},
				[2]string{"gateway", "transcode"}, [2]string{"transcode", "blobstore"}),
		},
	}
}
