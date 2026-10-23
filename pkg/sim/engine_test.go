package sim

import (
	"reflect"
	"testing"
)

func mustRun(t *testing.T, s *Scenario, trace []TraceTask, cfg Config) *Result {
	t.Helper()
	res, err := Run(s, trace, cfg, RunOptions{TieBreakSeed: 1})
	if err != nil {
		t.Fatalf("run %s: %v", cfg.Name, err)
	}
	return res
}

func defaultTrace(t *testing.T, s *Scenario) []TraceTask {
	t.Helper()
	trace, err := GenerateTrace(s, DefaultTraceParams())
	if err != nil {
		t.Fatal(err)
	}
	return trace
}

func nodesOf(res *Result) []string {
	out := make([]string, len(res.Placements))
	for i, p := range res.Placements {
		out[i] = p.Task.ID + "@" + p.Node
	}
	return out
}

func TestRunPlacesEveryTaskInAllConfigs(t *testing.T) {
	s := DefaultScenario()
	trace := defaultTrace(t, s)
	for _, cfg := range Configs() {
		if got := len(mustRun(t, s, trace, cfg).Placements); got != len(trace) {
			t.Errorf("%s: placed %d of %d tasks", cfg.Name, got, len(trace))
		}
	}
}

// Replays the placements and checks no node ever exceeds its cores or memory.
func TestRunNeverExceedsCapacity(t *testing.T) {
	s := DefaultScenario()
	trace := defaultTrace(t, s)
	for _, cfg := range Configs() {
		res := mustRun(t, s, trace, cfg)
		for _, probe := range res.Placements {
			// Load on a node at the instant probe starts.
			var cores float64
			var mem int64
			for _, p := range res.Placements {
				if p.Node == probe.Node && p.StartSec <= probe.StartSec && p.EndSec > probe.StartSec {
					cores += p.Task.CPU
					mem += p.Task.MemoryBytes
				}
			}
			spec, _ := s.Node(probe.Node)
			if cores > float64(spec.Cores) || mem > spec.MemoryKB*1024 {
				t.Fatalf("%s: %s over capacity at t=%v: %v cores, %d bytes", cfg.Name, probe.Node, probe.StartSec, cores, mem)
			}
		}
	}
}

func TestRunStartsDependenciesBeforeDependents(t *testing.T) {
	s := DefaultScenario()
	res := mustRun(t, s, defaultTrace(t, s), Configs()[3])

	start := map[string]float64{}
	for _, p := range res.Placements {
		start[p.Task.Instance+"/"+p.Task.Service] = p.StartSec
	}
	for _, p := range res.Placements {
		g, _ := s.AppGroup(p.Task.App)
		for _, e := range g.GetDependencies(p.Task.Service) {
			if start[p.Task.Instance+"/"+e.To] > p.StartSec {
				t.Fatalf("%s started before its dependency %s", p.Task.ID, e.To)
			}
		}
	}
}

func TestRunIsDeterministic(t *testing.T) {
	s := DefaultScenario()
	trace := defaultTrace(t, s)
	for _, cfg := range Configs() {
		a := nodesOf(mustRun(t, s, trace, cfg))
		b := nodesOf(mustRun(t, s, trace, cfg))
		if !reflect.DeepEqual(a, b) {
			t.Fatalf("%s: repeated runs placed tasks differently", cfg.Name)
		}
	}
}

// A cluster too small for the burst must queue work, not fail or overcommit.
func TestRunQueuesWhenClusterIsFull(t *testing.T) {
	s := DefaultScenario()
	for i := range s.Nodes {
		s.Nodes[i].Cores = 2
	}
	p := DefaultTraceParams()
	p.MeanInterarrival = 1 // heavy burst
	trace, _ := GenerateTrace(s, p)

	res := mustRun(t, s, trace, Configs()[0])
	if len(res.Placements) != len(trace) {
		t.Fatalf("placed %d of %d", len(res.Placements), len(trace))
	}
	waited := 0
	for _, pl := range res.Placements {
		if pl.QueueWaitSec > 0 {
			waited++
		}
	}
	if waited == 0 {
		t.Fatal("expected some tasks to queue on a saturated cluster")
	}
}

func TestRunFailsWhenTaskCanNeverFit(t *testing.T) {
	s := DefaultScenario()
	trace := []TraceTask{{
		ID: "shop-0/db", Instance: "shop-0", App: "shop", Service: "db",
		CPU: 64, MemoryBytes: 1 << 30, DurationSec: 10, DeadlineSec: 100, Class: ClassDefault,
	}}
	if _, err := Run(s, trace, Configs()[0], RunOptions{}); err == nil {
		t.Fatal("expected an error for a task larger than any node")
	}
}
