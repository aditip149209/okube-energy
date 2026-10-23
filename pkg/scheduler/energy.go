package scheduler

import (
	"math"

	"github.com/aditip149209/okube/pkg/node"
	"github.com/aditip149209/okube/pkg/task"
)

const (
	defaultEnergyWeight = 0.2
)

// applyEnergyAwareScore adds a carbon-weighted energy cost to the scores
// produced so far. Both the incoming scores and the raw energy costs are
// min-max normalised to 0-100 across the candidate set first: the network
// stage already emits 0-100 scores, but with no topology data the scores are
// raw resource costs (e.g. 0.1/1.0 for round-robin), so without this the
// effective energy weight would depend on which signals happened to be present.
// A zero weight returns the incoming scores untouched so a network-only
// baseline stays exact.
func applyEnergyAwareScore(t task.Task, nodes []*node.Node, baseScores map[string]float64, scoreCtx *ScoreContext) map[string]float64 {
	if len(nodes) == 0 || len(baseScores) == 0 {
		return baseScores
	}

	energyWeight := resolveEnergyWeight(scoreCtx, &t)
	if energyWeight == 0 {
		return baseScores
	}

	energyCosts := make(map[string]float64, len(nodes))
	cleanBase := make(map[string]float64, len(baseScores))
	for _, n := range nodes {
		if n == nil {
			continue
		}
		energyCosts[n.Name] = estimateNodeEnergyCost(n, &t)
		base := baseScores[n.Name]
		if math.IsNaN(base) || math.IsInf(base, 0) {
			base = 1e9
		}
		cleanBase[n.Name] = base
	}

	normBase := normalizeScores(nodes, cleanBase)
	normEnergy := normalizeScores(nodes, energyCosts)

	final := make(map[string]float64, len(baseScores))
	for _, n := range nodes {
		if n == nil {
			continue
		}
		final[n.Name] = normBase[n.Name] + energyWeight*normEnergy[n.Name]
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
