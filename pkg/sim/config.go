package sim

import "github.com/aditip149209/okube/pkg/scheduler"

// Config selects which scoring stages are active for one simulation run. All
// configs go through the same scheduler code; only the ScoreContext differs.
type Config struct {
	Name string

	DisableNetworkScore bool
	DisableEnergyScore  bool

	// Optional weight overrides; nil falls back to the scheduler defaults.
	NetworkWeight  *float64
	ResourceWeight *float64
	EnergyWeight   *float64
}

const (
	ConfigDefault     = "default"
	ConfigNetworkOnly = "network-only"
	ConfigEnergyOnly  = "energy-only"
	ConfigFused       = "fused"
)

// Configs returns the four evaluation configurations in report order.
func Configs() []Config {
	return []Config{
		{Name: ConfigDefault, DisableNetworkScore: true, DisableEnergyScore: true},
		{Name: ConfigNetworkOnly, DisableEnergyScore: true},
		{Name: ConfigEnergyOnly, DisableNetworkScore: true},
		{Name: ConfigFused},
	}
}

// WithEnergyWeight returns a copy of c with an explicit energy weight, for
// weight sweeps. An explicit zero is honoured, not treated as unset.
func (c Config) WithEnergyWeight(w float64) Config {
	c.EnergyWeight = &w
	return c
}

// WithNetworkWeight returns a copy of c with an explicit network weight.
func (c Config) WithNetworkWeight(w float64) Config {
	c.NetworkWeight = &w
	return c
}

// NetworkAware reports whether the network stages are active. When they are
// off, the simulator withholds the filter context entirely: the network
// filter is not controlled by DisableNetworkScore and would otherwise still
// enforce edge constraints, making the baselines not truly network-blind.
func (c Config) NetworkAware() bool {
	return !c.DisableNetworkScore
}

// ScoreContext builds the scheduler's ScoreContext for this config. filter is
// the per-task filter context; it is dropped when the config is not network
// aware.
func (c Config) ScoreContext(filter *scheduler.FilterContext) *scheduler.ScoreContext {
	if !c.NetworkAware() {
		filter = nil
	}
	return &scheduler.ScoreContext{
		Filter:              filter,
		NetworkWeight:       c.NetworkWeight,
		ResourceWeight:      c.ResourceWeight,
		EnergyWeight:        c.EnergyWeight,
		DisableNetworkScore: c.DisableNetworkScore,
		DisableEnergyScore:  c.DisableEnergyScore,
	}
}

// FilterContextFor returns the filter context to pass to SelectCandidateNodes:
// the given one for network-aware configs, nil otherwise.
func (c Config) FilterContextFor(filter *scheduler.FilterContext) *scheduler.FilterContext {
	if !c.NetworkAware() {
		return nil
	}
	return filter
}

// NewScheduler builds a fresh scheduler for a run. The scheduler is stateful
// for some resource scorers, so each run needs its own instance. It uses the
// deterministic, offline "leastloaded" resource scorer.
func NewScheduler() scheduler.Scheduler {
	return scheduler.NewPipelineScheduler("leastloaded", "")
}
