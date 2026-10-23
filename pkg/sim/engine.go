package sim

import (
	"fmt"
	"hash/fnv"
	"math"
	"math/rand"
	"time"

	"github.com/aditip149209/okube/pkg/node"
	"github.com/aditip149209/okube/pkg/scheduler"
	"github.com/aditip149209/okube/pkg/task"
)

// Placement records where and when one task ran.
type Placement struct {
	Task         TraceTask
	Node         string
	StartSec     float64
	EndSec       float64
	QueueWaitSec float64

	// HasDependencies is false for root services, which the network stage
	// never scores; DependencyLatencyMS is then meaningless.
	HasDependencies     bool
	DependencyLatencyMS float64 // sum of latency to each dependency's node (Eq. 1)

	Decision time.Duration // wall-clock time of filter + score + pick
}

// Result is the raw output of one simulation run.
type Result struct {
	Config      string
	Placements  []Placement // in scheduling order
	MakespanSec float64
}

// RunOptions holds run-level knobs that are not part of the evaluated config.
type RunOptions struct {
	// TieBreakSeed seeds a per-task shuffle of the candidate order. The
	// scheduler breaks score ties by taking the first candidate, so without
	// shuffling, ties would always go to the first node in the scenario (the
	// cleanest zone) and quietly make every baseline carbon-aware.
	TieBreakSeed int64
}

type nodeState struct {
	spec         NodeSpec
	usedCores    float64
	usedMemBytes int64
	tasks        int
}

func (n *nodeState) fits(tk TraceTask) bool {
	return n.usedCores+tk.CPU <= float64(n.spec.Cores) &&
		n.usedMemBytes+tk.MemoryBytes <= n.spec.MemoryKB*1024
}

type activeTask struct {
	endSec float64
	node   *nodeState
	cpu    float64
	mem    int64
}

// Run simulates the trace on the scenario under one config. Tasks go through
// the real scheduler pipeline (filter, score, pick); the simulator only
// supplies the cluster state around it. Tasks that do not fit anywhere wait in
// a FIFO queue until capacity frees, and a service waits until the services it
// depends on have been placed, as the manager's DeployApp does.
func Run(s *Scenario, trace []TraceTask, cfg Config, opts RunOptions) (*Result, error) {
	if err := s.Validate(); err != nil {
		return nil, fmt.Errorf("invalid scenario: %w", err)
	}

	sched := NewScheduler()
	states := make([]*nodeState, len(s.Nodes))
	for i, spec := range s.Nodes {
		states[i] = &nodeState{spec: spec}
	}

	var (
		now      float64
		next     int
		pending  []TraceTask
		active   []activeTask
		placedAt = map[string]map[string]string{} // instance -> service -> node
		res      = &Result{Config: cfg.Name}
	)

	for next < len(trace) || len(pending) > 0 || len(active) > 0 {
		t := math.Inf(1)
		if next < len(trace) {
			t = trace[next].ArrivalSec
		}
		for _, a := range active {
			t = math.Min(t, a.endSec)
		}
		if math.IsInf(t, 1) {
			return nil, fmt.Errorf("task %s can never be placed: no capacity and nothing running", pending[0].ID)
		}
		now = t

		// Completions free capacity before new decisions at the same instant.
		live := active[:0]
		for _, a := range active {
			if a.endSec <= now {
				a.node.usedCores -= a.cpu
				a.node.usedMemBytes -= a.mem
				a.node.tasks--
				continue
			}
			live = append(live, a)
		}
		active = live

		for next < len(trace) && trace[next].ArrivalSec <= now {
			pending = append(pending, trace[next])
			next++
		}

		var stillPending []TraceTask
		for _, tk := range pending {
			group, ok := s.AppGroup(tk.App)
			if !ok {
				return nil, fmt.Errorf("task %s: unknown app %q", tk.ID, tk.App)
			}
			if placedAt[tk.Instance] == nil {
				placedAt[tk.Instance] = map[string]string{}
			}
			deps := group.GetDependencies(tk.Service)

			ready := true
			for _, e := range deps {
				if _, done := placedAt[tk.Instance][e.To]; !done {
					ready = false
					break
				}
			}
			if !ready {
				stillPending = append(stillPending, tk)
				continue
			}

			candidates := candidateNodes(states, tk, opts.TieBreakSeed)
			if len(candidates) == 0 {
				stillPending = append(stillPending, tk)
				continue
			}

			filterCtx := &scheduler.FilterContext{
				AppGroup:                group,
				NetworkTopology:         s.Topology,
				DependencyNodeByService: placedAt[tk.Instance],
			}
			st := toSchedulerTask(tk)

			began := time.Now()
			filtered := sched.SelectCandidateNodes(st, candidates, cfg.FilterContextFor(filterCtx))
			if len(filtered) == 0 {
				stillPending = append(stillPending, tk)
				continue
			}
			scores := sched.Score(st, filtered, cfg.ScoreContext(filterCtx))
			picked := sched.Pick(scores, filtered)
			decision := time.Since(began)
			if picked == nil {
				return nil, fmt.Errorf("scheduler picked no node for %s", tk.ID)
			}

			chosen := stateByName(states, picked.Name)
			chosen.usedCores += tk.CPU
			chosen.usedMemBytes += tk.MemoryBytes
			chosen.tasks++
			end := now + tk.DurationSec
			active = append(active, activeTask{endSec: end, node: chosen, cpu: tk.CPU, mem: tk.MemoryBytes})
			placedAt[tk.Instance][tk.Service] = chosen.spec.Name

			p := Placement{
				Task: tk, Node: chosen.spec.Name,
				StartSec: now, EndSec: end, QueueWaitSec: now - tk.ArrivalSec,
				HasDependencies: len(deps) > 0,
				Decision:        decision,
			}
			for _, e := range deps {
				lat, _ := s.Topology.GetLatency(chosen.spec.Name, placedAt[tk.Instance][e.To])
				p.DependencyLatencyMS += lat
			}
			res.Placements = append(res.Placements, p)
			res.MakespanSec = math.Max(res.MakespanSec, end)
		}
		pending = stillPending
	}

	return res, nil
}

// candidateNodes builds scheduler-facing nodes for every node with room for
// the task, in a per-task deterministic shuffled order (see RunOptions).
func candidateNodes(states []*nodeState, tk TraceTask, seed int64) []*node.Node {
	var out []*node.Node
	for _, st := range states {
		if !st.fits(tk) {
			continue
		}
		out = append(out, &node.Node{
			Name:            st.spec.Name,
			Cores:           st.spec.Cores,
			Memory:          st.spec.MemoryKB,
			MemoryAllocated: int(st.usedMemBytes / 1024),
			TaskCount:       st.tasks,
			CarbonIntensity: st.spec.CarbonFactor(),
		})
	}

	h := fnv.New64a()
	h.Write([]byte(tk.ID))
	rng := rand.New(rand.NewSource(seed ^ int64(h.Sum64())))
	rng.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}

func stateByName(states []*nodeState, name string) *nodeState {
	for _, st := range states {
		if st.spec.Name == name {
			return st
		}
	}
	return nil
}

// simEpoch anchors the trace's relative seconds to wall-clock deadlines for
// the scheduler's task metadata.
var simEpoch = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func toSchedulerTask(tk TraceTask) task.Task {
	t := task.Task{
		Name:              tk.ID,
		AppID:             tk.Instance,
		ServiceID:         tk.Service,
		Cpu:               tk.CPU,
		Memory:            int(tk.MemoryBytes),
		EstimatedDuration: int(tk.DurationSec),
		Deadline:          simEpoch.Add(time.Duration(tk.DeadlineSec * float64(time.Second))),
	}
	switch tk.Class {
	case ClassCritical:
		t.Criticality = "critical"
	case ClassFlexible:
		t.Flexible = true
	}
	return t
}
