package sim

import (
	"math"
	"sort"
	"time"
)

const joulesPerKWh = 3.6e6

// ClassStats are the per-class slice of a Summary.
type ClassStats struct {
	Tasks              int
	DynamicCO2G        float64
	MeanIntensityGKWh  float64 // average grid intensity of the nodes used
	DeadlineViolations int
}

// Summary is the report-facing view of one run.
type Summary struct {
	Config      string
	Tasks       int
	MakespanSec float64

	// Energy and CO2. Dynamic is the part the scheduler can influence: with a
	// linear power model it is (max-idle) * cpu/cores * duration per task. Idle
	// is every node's idle draw over the makespan, which no placement avoids,
	// so TotalCO2G mostly dilutes the differences between configs.
	DynamicEnergyKWh float64
	IdleEnergyKWh    float64
	DynamicCO2G      float64
	IdleCO2G         float64
	TotalCO2G        float64

	// Latency is Eq. 1 over services that have dependencies.
	LatencyTasks  int
	MeanLatencyMS float64
	P95LatencyMS  float64

	DeadlineViolations    int
	DeadlineViolationRate float64
	MeanQueueWaitSec      float64

	MeanDecisionUS float64
	P95DecisionUS  float64

	TasksPerNode map[string]int
	ByClass      map[Class]ClassStats
}

// Summarize computes the evaluation metrics for a run.
func Summarize(s *Scenario, res *Result) Summary {
	sum := Summary{
		Config:       res.Config,
		Tasks:        len(res.Placements),
		MakespanSec:  res.MakespanSec,
		TasksPerNode: map[string]int{},
		ByClass:      map[Class]ClassStats{},
	}

	var latencies, decisionsUS []float64
	var latencySum, waitSum, decisionSumUS float64
	intensitySum := map[Class]float64{}

	for _, p := range res.Placements {
		spec, _ := s.Node(p.Node)

		dynKWh := (spec.MaxWatts - spec.IdleWatts) * (p.Task.CPU / float64(spec.Cores)) * p.Task.DurationSec / joulesPerKWh
		dynG := dynKWh * spec.CarbonGPerKWh
		sum.DynamicEnergyKWh += dynKWh
		sum.DynamicCO2G += dynG

		sum.TasksPerNode[p.Node]++
		waitSum += p.QueueWaitSec

		violated := p.EndSec > p.Task.DeadlineSec
		if violated {
			sum.DeadlineViolations++
		}

		cs := sum.ByClass[p.Task.Class]
		cs.Tasks++
		cs.DynamicCO2G += dynG
		intensitySum[p.Task.Class] += spec.CarbonGPerKWh
		if violated {
			cs.DeadlineViolations++
		}
		sum.ByClass[p.Task.Class] = cs

		if p.HasDependencies {
			latencies = append(latencies, p.DependencyLatencyMS)
			latencySum += p.DependencyLatencyMS
		}

		us := float64(p.Decision) / float64(time.Microsecond)
		decisionsUS = append(decisionsUS, us)
		decisionSumUS += us
	}

	for _, n := range s.Nodes {
		idleKWh := n.IdleWatts * res.MakespanSec / joulesPerKWh
		sum.IdleEnergyKWh += idleKWh
		sum.IdleCO2G += idleKWh * n.CarbonGPerKWh
	}
	sum.TotalCO2G = sum.DynamicCO2G + sum.IdleCO2G

	for c, cs := range sum.ByClass {
		cs.MeanIntensityGKWh = intensitySum[c] / float64(cs.Tasks)
		sum.ByClass[c] = cs
	}

	if n := len(res.Placements); n > 0 {
		sum.DeadlineViolationRate = float64(sum.DeadlineViolations) / float64(n)
		sum.MeanQueueWaitSec = waitSum / float64(n)
		sum.MeanDecisionUS = decisionSumUS / float64(n)
		sum.P95DecisionUS = percentile(decisionsUS, 0.95)
	}
	sum.LatencyTasks = len(latencies)
	if len(latencies) > 0 {
		sum.MeanLatencyMS = latencySum / float64(len(latencies))
		sum.P95LatencyMS = percentile(latencies, 0.95)
	}
	return sum
}

// percentile returns the nearest-rank p-th percentile (p in (0,1]) of values.
func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	rank := int(math.Ceil(p*float64(len(sorted)))) - 1
	if rank < 0 {
		rank = 0
	}
	return sorted[rank]
}
