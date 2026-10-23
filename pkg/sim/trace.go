package sim

import (
	"fmt"
	"math/rand"

	"github.com/aditip149209/okube/pkg/appgroup"
)

// Class is a workload's scheduling class, mirroring task.Task's Criticality
// and Flexible fields.
type Class string

const (
	ClassCritical Class = "critical"
	ClassFlexible Class = "flexible"
	ClassDefault  Class = "default"
)

// TraceTask is one service instance to be scheduled. Times are seconds from
// the start of the simulation.
type TraceTask struct {
	ID          string // unique per task
	Instance    string // unique per deployed copy of an app, e.g. "shop-3"
	App         string // app group template, e.g. "shop"
	Service     string
	ArrivalSec  float64
	DurationSec float64
	CPU         float64 // requested cores
	MemoryBytes int64
	Class       Class
	DeadlineSec float64 // absolute, from simulation start
}

// ClassMix gives the relative probability of each class. It need not sum to 1.
type ClassMix struct {
	Critical float64
	Flexible float64
	Default  float64
}

// TraceParams controls trace generation. The same params (including Seed)
// always produce the identical trace.
type TraceParams struct {
	Seed             int64
	Instances        int     // number of app deployments
	MeanInterarrival float64 // seconds between deployments (exponential)
	Mix              ClassMix
	MinDurationSec   float64
	MaxDurationSec   float64

	// DeadlineSlack multiplies a task's duration to give the time allowed
	// after arrival, per class. Critical work is tight, flexible work loose.
	CriticalSlack float64
	DefaultSlack  float64
	FlexibleSlack float64
}

// DefaultTraceParams returns the base experiment's workload: 60 deployments,
// a mix leaning towards flexible work so the energy term has room to act.
func DefaultTraceParams() TraceParams {
	return TraceParams{
		Seed:             42,
		Instances:        60,
		MeanInterarrival: 30,
		Mix:              ClassMix{Critical: 0.2, Flexible: 0.4, Default: 0.4},
		MinDurationSec:   60,
		MaxDurationSec:   600,
		CriticalSlack:    1.5,
		DefaultSlack:     3,
		FlexibleSlack:    6,
	}
}

var (
	cpuChoices    = []float64{0.5, 1, 2}
	memoryChoices = []int64{512 << 20, 1 << 30, 2 << 30}
)

// GenerateTrace builds a deterministic workload for the scenario. Each
// instance is one deployment of a randomly chosen app group; every service in
// it arrives at the same time and shares the instance's class and duration.
// Within an instance, tasks are emitted dependencies first, because the
// scheduler's network stage can only score a service against dependencies that
// are already placed.
func GenerateTrace(s *Scenario, p TraceParams) ([]TraceTask, error) {
	if err := s.Validate(); err != nil {
		return nil, fmt.Errorf("invalid scenario: %w", err)
	}
	if p.Instances <= 0 || p.MeanInterarrival <= 0 {
		return nil, fmt.Errorf("instances and mean interarrival must be positive")
	}
	if p.MinDurationSec <= 0 || p.MaxDurationSec < p.MinDurationSec {
		return nil, fmt.Errorf("need 0 < min duration <= max duration")
	}
	total := p.Mix.Critical + p.Mix.Flexible + p.Mix.Default
	if total <= 0 || p.Mix.Critical < 0 || p.Mix.Flexible < 0 || p.Mix.Default < 0 {
		return nil, fmt.Errorf("class mix must be non-negative with a positive sum")
	}

	rng := rand.New(rand.NewSource(p.Seed))
	var tasks []TraceTask
	clock := 0.0

	for i := 0; i < p.Instances; i++ {
		clock += rng.ExpFloat64() * p.MeanInterarrival
		group := s.AppGroups[rng.Intn(len(s.AppGroups))]
		class := pickClass(rng.Float64()*total, p.Mix)
		duration := p.MinDurationSec + rng.Float64()*(p.MaxDurationSec-p.MinDurationSec)
		instance := fmt.Sprintf("%s-%d", group.AppID, i)

		order, err := dependenciesFirst(group)
		if err != nil {
			return nil, err
		}
		for _, svc := range order {
			tasks = append(tasks, TraceTask{
				ID:          fmt.Sprintf("%s/%s", instance, svc),
				Instance:    instance,
				App:         group.AppID,
				Service:     svc,
				ArrivalSec:  clock,
				DurationSec: duration,
				CPU:         cpuChoices[rng.Intn(len(cpuChoices))],
				MemoryBytes: memoryChoices[rng.Intn(len(memoryChoices))],
				Class:       class,
				DeadlineSec: clock + duration*slackFor(class, p),
			})
		}
	}
	return tasks, nil
}

func pickClass(x float64, m ClassMix) Class {
	switch {
	case x < m.Critical:
		return ClassCritical
	case x < m.Critical+m.Flexible:
		return ClassFlexible
	default:
		return ClassDefault
	}
}

func slackFor(c Class, p TraceParams) float64 {
	switch c {
	case ClassCritical:
		return p.CriticalSlack
	case ClassFlexible:
		return p.FlexibleSlack
	default:
		return p.DefaultSlack
	}
}

// dependenciesFirst returns the group's services so every service appears
// after the services it depends on. appgroup.TopologicalOrder returns
// dependents first, so it is reversed here.
func dependenciesFirst(g *appgroup.AppGroup) ([]string, error) {
	order, err := g.TopologicalOrder()
	if err != nil {
		return nil, err
	}
	out := make([]string, len(order))
	for i, svc := range order {
		out[len(order)-1-i] = svc
	}
	return out, nil
}
