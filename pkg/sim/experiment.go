package sim

import (
	"fmt"
	"math"
	"strconv"
)

// WeightDefault labels runs that use the scheduler's default energy weight.
const WeightDefault = "default"

// Workload is a named trace parameter set, e.g. the base load or the stress load.
type Workload struct {
	Name   string
	Params TraceParams
}

// ExperimentOptions describes the full comparison: every config on every
// workload over several seeds, plus an energy-weight sweep.
type ExperimentOptions struct {
	Seeds     int
	FirstSeed int64 // seeds run are FirstSeed .. FirstSeed+Seeds-1; 0 means 1

	Workloads    []Workload
	SweepWeights []float64 // applied to energy-only and fused
}

// DefaultExperimentOptions is the experiment reported in the paper.
func DefaultExperimentOptions() ExperimentOptions {
	return ExperimentOptions{
		Seeds: 20,
		Workloads: []Workload{
			{Name: "default", Params: DefaultTraceParams()},
			{Name: "stress", Params: StressTraceParams()},
		},
		SweepWeights: []float64{0, 0.1, 0.2, 0.5, 1, 2, 5},
	}
}

// RunRow is one simulation: a workload, config, energy weight and seed.
type RunRow struct {
	Workload     string
	Config       string
	EnergyWeight string // WeightDefault or the explicit weight
	Seed         int64
	Summary      Summary
}

// RunExperiment runs every (workload, seed) trace under the four configs at
// default weights, then energy-only and fused at each sweep weight. A seed
// fixes both the trace and the tie-break order, so every config in a seed sees
// the identical workload and the comparisons can be paired.
func RunExperiment(s *Scenario, opts ExperimentOptions) ([]RunRow, error) {
	if opts.Seeds <= 0 {
		return nil, fmt.Errorf("seeds must be positive")
	}
	if len(opts.Workloads) == 0 {
		return nil, fmt.Errorf("no workloads")
	}

	first := opts.FirstSeed
	if first == 0 {
		first = 1
	}

	var rows []RunRow
	for _, w := range opts.Workloads {
		for seed := first; seed < first+int64(opts.Seeds); seed++ {
			params := w.Params
			params.Seed = seed
			trace, err := GenerateTrace(s, params)
			if err != nil {
				return nil, fmt.Errorf("workload %s seed %d: %w", w.Name, seed, err)
			}

			run := func(cfg Config, weight string) error {
				res, err := Run(s, trace, cfg, RunOptions{TieBreakSeed: seed})
				if err != nil {
					return fmt.Errorf("workload %s seed %d config %s: %w", w.Name, seed, cfg.Name, err)
				}
				rows = append(rows, RunRow{
					Workload: w.Name, Config: cfg.Name, EnergyWeight: weight,
					Seed: seed, Summary: Summarize(s, res),
				})
				return nil
			}

			for _, cfg := range Configs() {
				if err := run(cfg, WeightDefault); err != nil {
					return nil, err
				}
			}
			for _, name := range []string{ConfigEnergyOnly, ConfigFused} {
				for _, weight := range opts.SweepWeights {
					cfg := configByName(name).WithEnergyWeight(weight)
					if err := run(cfg, formatWeight(weight)); err != nil {
						return nil, err
					}
				}
			}
		}
	}
	return rows, nil
}

func configByName(name string) Config {
	for _, c := range Configs() {
		if c.Name == name {
			return c
		}
	}
	panic("sim: unknown config " + name)
}

func formatWeight(w float64) string {
	return strconv.FormatFloat(w, 'g', -1, 64)
}

// Stat is a mean and sample standard deviation over seeds.
type Stat struct {
	Mean float64
	Std  float64
}

func newStat(values []float64) Stat {
	n := float64(len(values))
	if n == 0 {
		return Stat{}
	}
	var sum float64
	for _, v := range values {
		sum += v
	}
	mean := sum / n
	if n < 2 {
		return Stat{Mean: mean}
	}
	var sq float64
	for _, v := range values {
		sq += (v - mean) * (v - mean)
	}
	return Stat{Mean: mean, Std: math.Sqrt(sq / (n - 1))}
}

// AggRow aggregates the runs of one (workload, config, weight) over seeds.
type AggRow struct {
	Workload     string
	Config       string
	EnergyWeight string
	Runs         int

	DynamicCO2G       Stat
	TotalCO2G         Stat
	MeanLatencyMS     Stat
	P95LatencyMS      Stat
	ViolationRate     Stat
	MeanQueueWaitSec  Stat
	MeanDecisionUS    Stat
	FlexibleIntensity Stat // grid g/kWh of nodes used by flexible tasks
	CriticalIntensity Stat

	// Percentage change against the default config on the same workload and
	// seed (negative is better), averaged over seeds. Paired because every
	// config in a seed runs the identical trace.
	CO2VsDefaultPct     Stat
	LatencyVsDefaultPct Stat
}

type groupKey struct{ workload, config, weight string }

// Aggregate groups runs by (workload, config, weight), preserving first-seen
// order, and computes per-metric statistics over seeds.
func Aggregate(rows []RunRow) []AggRow {
	var order []groupKey
	groups := map[groupKey][]RunRow{}
	for _, r := range rows {
		k := groupKey{r.Workload, r.Config, r.EnergyWeight}
		if _, seen := groups[k]; !seen {
			order = append(order, k)
		}
		groups[k] = append(groups[k], r)
	}

	type seedKey struct {
		workload string
		seed     int64
	}
	defaults := map[seedKey]Summary{}
	for _, r := range rows {
		if r.Config == ConfigDefault && r.EnergyWeight == WeightDefault {
			defaults[seedKey{r.Workload, r.Seed}] = r.Summary
		}
	}

	col := func(rs []RunRow, f func(Summary) float64) Stat {
		vals := make([]float64, len(rs))
		for i, r := range rs {
			vals[i] = f(r.Summary)
		}
		return newStat(vals)
	}
	paired := func(rs []RunRow, f func(Summary) float64) Stat {
		var vals []float64
		for _, r := range rs {
			base, ok := defaults[seedKey{r.Workload, r.Seed}]
			if !ok || f(base) == 0 {
				continue
			}
			vals = append(vals, (f(r.Summary)-f(base))/f(base)*100)
		}
		return newStat(vals)
	}

	out := make([]AggRow, 0, len(order))
	for _, k := range order {
		rs := groups[k]
		out = append(out, AggRow{
			Workload: k.workload, Config: k.config, EnergyWeight: k.weight, Runs: len(rs),

			DynamicCO2G:       col(rs, func(s Summary) float64 { return s.DynamicCO2G }),
			TotalCO2G:         col(rs, func(s Summary) float64 { return s.TotalCO2G }),
			MeanLatencyMS:     col(rs, func(s Summary) float64 { return s.MeanLatencyMS }),
			P95LatencyMS:      col(rs, func(s Summary) float64 { return s.P95LatencyMS }),
			ViolationRate:     col(rs, func(s Summary) float64 { return s.DeadlineViolationRate }),
			MeanQueueWaitSec:  col(rs, func(s Summary) float64 { return s.MeanQueueWaitSec }),
			MeanDecisionUS:    col(rs, func(s Summary) float64 { return s.MeanDecisionUS }),
			FlexibleIntensity: col(rs, func(s Summary) float64 { return s.ByClass[ClassFlexible].MeanIntensityGKWh }),
			CriticalIntensity: col(rs, func(s Summary) float64 { return s.ByClass[ClassCritical].MeanIntensityGKWh }),

			CO2VsDefaultPct:     paired(rs, func(s Summary) float64 { return s.DynamicCO2G }),
			LatencyVsDefaultPct: paired(rs, func(s Summary) float64 { return s.MeanLatencyMS }),
		})
	}
	return out
}
