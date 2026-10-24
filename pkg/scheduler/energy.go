package scheduler

import (
	"math"

	"github.com/aditip149209/okube/pkg/node"
	"github.com/aditip149209/okube/pkg/task"
)

const (
	defaultEnergyWeight = 0.2

	// defaultCarbonFactor is assumed for a node with no carbon data (nil
	// CarbonIntensity), roughly a mid-carbon grid. It is never applied to a
	// real intensity of 0, which is a fully low-carbon grid.
	defaultCarbonFactor = 0.5

	// refCarbonFactor is the intensity (unitless, 0.3 = 300 gCO2/kWh) at and
	// above which a grid counts as maximally carbon-intensive for scoring. It
	// is roughly the dirty end of GB regional intensities.
	refCarbonFactor = 0.3

	// refLoadFactor is the load multiplier at and above which the load term
	// saturates. An idle node with a small task has a load factor near 1.
	refLoadFactor = 2.0

	// maxEnergyCost is the energy cost on the same 0-100 scale as the
	// normalised network and resource scores.
	maxEnergyCost = 100.0
)

// applyEnergyAwareScore adds a carbon-weighted energy cost to the scores
// produced so far.
//
// The incoming scores are min-max normalised to 0-100 across the candidate set
// so the energy weight means the same thing whether they came from the network
// stage (already 0-100) or from raw resource costs (e.g. 0.1/1.0 for
// round-robin). The energy cost is deliberately NOT normalised against the
// candidates: it is measured on a fixed scale (see estimateNodeEnergyCost), so
// a spread between uniformly clean grids stays small instead of being
// stretched to the full range. A zero weight returns the incoming scores
// untouched so a network-only baseline stays exact.
func applyEnergyAwareScore(t task.Task, nodes []*node.Node, baseScores map[string]float64, scoreCtx *ScoreContext) map[string]float64 {
	if len(nodes) == 0 || len(baseScores) == 0 {
		return baseScores
	}

	energyWeight := resolveEnergyWeight(scoreCtx, &t)
	if energyWeight == 0 {
		return baseScores
	}

	cleanBase := make(map[string]float64, len(baseScores))
	for _, n := range nodes {
		if n == nil {
			continue
		}
		base := baseScores[n.Name]
		if math.IsNaN(base) || math.IsInf(base, 0) {
			base = 1e9
		}
		cleanBase[n.Name] = base
	}
	normBase := normalizeScores(nodes, cleanBase)

	final := make(map[string]float64, len(baseScores))
	for _, n := range nodes {
		if n == nil {
			continue
		}
		final[n.Name] = normBase[n.Name] + energyWeight*estimateNodeEnergyCost(n, &t)
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

// estimateNodeEnergyCost scores how carbon-costly it is to run t on n, on a
// fixed 0-100 scale:
//
//	cost = 100 * min(1, carbon/refCarbonFactor) * min(1, load/refLoadFactor)
//
// Zero carbon gives zero cost; an unknown carbon intensity falls back to
// defaultCarbonFactor. load is 1 plus the task's share of the node's memory and
// cores plus 0.08 per task already on the node.
func estimateNodeEnergyCost(n *node.Node, t *task.Task) float64 {
	if n == nil {
		return 0
	}
	carbonFactor := defaultCarbonFactor
	if n.CarbonIntensity != nil && *n.CarbonIntensity >= 0 {
		carbonFactor = *n.CarbonIntensity
	}

	loadFactor := 1.0
	if t != nil {
		// Task memory is in bytes (Docker), node memory in KB (/proc/meminfo).
		if t.Memory > 0 && n.Memory > 0 {
			loadFactor += (float64(t.Memory) / 1024) / float64(n.Memory)
		}
		if t.Cpu > 0 && n.Cores > 0 {
			loadFactor += t.Cpu / float64(n.Cores)
		}
	}
	if n.TaskCount > 0 {
		loadFactor += float64(n.TaskCount) * 0.08
	}

	return maxEnergyCost * math.Min(1, carbonFactor/refCarbonFactor) * math.Min(1, loadFactor/refLoadFactor)
}
