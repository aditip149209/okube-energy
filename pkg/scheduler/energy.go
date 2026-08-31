package scheduler

import (
	"math"

	"github.com/aditip149209/okube/pkg/node"
	"github.com/aditip149209/okube/pkg/task"
)

const (
	defaultEnergyWeight = 0.2
)

func applyEnergyAwareScore(t task.Task, nodes []*node.Node, baseScores map[string]float64, scoreCtx *ScoreContext) map[string]float64 {
	if len(nodes) == 0 || len(baseScores) == 0 {
		return baseScores
	}

	final := make(map[string]float64, len(baseScores))
	energyWeight := resolveEnergyWeight(scoreCtx, &t)
	for _, n := range nodes {
		if n == nil {
			continue
		}
		energyCost := estimateNodeEnergyCost(n, &t)
		base := baseScores[n.Name]
		if math.IsNaN(base) || math.IsInf(base, 0) {
			base = 1e9
		}
		final[n.Name] = base + (energyWeight * energyCost)
	}
	return final
}

func resolveEnergyWeight(scoreCtx *ScoreContext, t *task.Task) float64 {
	weight := defaultEnergyWeight
	if scoreCtx != nil && scoreCtx.EnergyWeight != nil {
		weight = *scoreCtx.EnergyWeight
	}
	if t != nil && (t.Criticality == "critical" || t.Criticality == "urgent") {
		return weight * 0.35
	}
	if t != nil && t.Flexible {
		return weight * 1.4
	}
	return weight
}

func estimateNodeEnergyCost(n *node.Node, t *task.Task) float64 {
	if n == nil {
		return 0
	}
	carbonFactor := n.CarbonIntensity
	if carbonFactor <= 0 {
		carbonFactor = 0.5
	}
	loadFactor := 1.0
	if t != nil {
		if t.Memory > 0 && n.Memory > 0 {
			loadFactor += float64(t.Memory) / float64(n.Memory)
		}
		if t.Cpu > 0 && n.Cores > 0 {
			loadFactor += float64(t.Cpu) / float64(n.Cores)
		}
	}
	if n.TaskCount > 0 {
		loadFactor += float64(n.TaskCount) * 0.08
	}
	return carbonFactor * loadFactor * 10.0
}
