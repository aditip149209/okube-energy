package sim

import (
	"reflect"
	"testing"
)

func TestGenerateTraceIsDeterministic(t *testing.T) {
	s := DefaultScenario()
	a, err := GenerateTrace(s, DefaultTraceParams())
	if err != nil {
		t.Fatal(err)
	}
	b, _ := GenerateTrace(s, DefaultTraceParams())
	if !reflect.DeepEqual(a, b) {
		t.Fatal("same seed produced different traces")
	}

	p := DefaultTraceParams()
	p.Seed = 7
	c, _ := GenerateTrace(s, p)
	if reflect.DeepEqual(a, c) {
		t.Fatal("different seeds produced identical traces")
	}
}

func TestGenerateTraceTasksAreWellFormed(t *testing.T) {
	s := DefaultScenario()
	tasks, err := GenerateTrace(s, DefaultTraceParams())
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) == 0 {
		t.Fatal("empty trace")
	}

	ids := map[string]struct{}{}
	prev := 0.0
	for _, tk := range tasks {
		if _, dup := ids[tk.ID]; dup {
			t.Fatalf("duplicate task id %s", tk.ID)
		}
		ids[tk.ID] = struct{}{}

		g, ok := s.AppGroup(tk.App)
		if !ok {
			t.Fatalf("task %s references unknown app %q", tk.ID, tk.App)
		}
		found := false
		for _, svc := range g.Services {
			found = found || svc == tk.Service
		}
		if !found {
			t.Fatalf("task %s references unknown service %q", tk.ID, tk.Service)
		}
		if tk.ArrivalSec < prev {
			t.Fatalf("arrivals not non-decreasing at %s", tk.ID)
		}
		prev = tk.ArrivalSec
		if tk.DeadlineSec <= tk.ArrivalSec+tk.DurationSec {
			t.Fatalf("task %s deadline %v leaves no slack after finish %v", tk.ID, tk.DeadlineSec, tk.ArrivalSec+tk.DurationSec)
		}
	}
}

// Within an instance every service must come after the services it depends on.
func TestGenerateTraceEmitsDependenciesFirst(t *testing.T) {
	s := DefaultScenario()
	tasks, _ := GenerateTrace(s, DefaultTraceParams())

	position := map[string]int{}
	for i, tk := range tasks {
		position[tk.Instance+"/"+tk.Service] = i
	}
	for _, tk := range tasks {
		g, _ := s.AppGroup(tk.App)
		for _, e := range g.GetDependencies(tk.Service) {
			if position[tk.Instance+"/"+e.To] >= position[tk.Instance+"/"+tk.Service] {
				t.Fatalf("%s emitted before its dependency %s", tk.ID, e.To)
			}
		}
	}
}

func TestGenerateTraceHonoursClassMix(t *testing.T) {
	p := DefaultTraceParams()
	p.Instances = 2000
	p.Mix = ClassMix{Critical: 1, Flexible: 0, Default: 0}
	tasks, _ := GenerateTrace(DefaultScenario(), p)
	for _, tk := range tasks {
		if tk.Class != ClassCritical {
			t.Fatalf("expected only critical tasks, got %s", tk.Class)
		}
	}

	p.Mix = ClassMix{Critical: 0.2, Flexible: 0.4, Default: 0.4}
	tasks, _ = GenerateTrace(DefaultScenario(), p)
	instances := map[string]Class{}
	counts := map[Class]int{}
	for _, tk := range tasks {
		if _, seen := instances[tk.Instance]; !seen {
			instances[tk.Instance] = tk.Class
			counts[tk.Class]++
		}
	}
	if share := float64(counts[ClassFlexible]) / float64(p.Instances); share < 0.35 || share > 0.45 {
		t.Fatalf("expected ~40%% flexible instances, got %.2f", share)
	}
}

func TestGenerateTraceRejectsBadParams(t *testing.T) {
	p := DefaultTraceParams()
	p.Mix = ClassMix{}
	if _, err := GenerateTrace(DefaultScenario(), p); err == nil {
		t.Fatal("expected empty class mix to be rejected")
	}
}
