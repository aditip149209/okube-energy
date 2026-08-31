# Network + Energy Aware Scheduler: Repo Status and Future Roadmap

## 1. Current state of the repository

This repository is a custom Go-based container orchestrator, not a Kubernetes scheduler plugin. The code already implements:

- a manager/worker architecture
- Docker-based task execution
- a network-aware custom scheduler
- etcd-backed coordination and leader election

Relevant implementation points:

- The scheduler pipeline is defined in [pkg/scheduler/pipeline.go](pkg/scheduler/pipeline.go)
- The network-aware score calculation lives in [pkg/scheduler/network_score.go](pkg/scheduler/network_score.go)
- The manager logic is in [pkg/manager/manager.go](pkg/manager/manager.go)
- The task metadata model is in [pkg/task/task.go](pkg/task/task.go)
- The node metadata model is in [pkg/node/node.go](pkg/node/node.go)

This system is therefore best understood as a prototype for network-aware placement inside a custom orchestrator, rather than a Kubernetes Scheduling Framework plugin.

## 2. What was added in this pass

The minimal energy-aware extension added in the repo includes:

- new task metadata for energy-aware scheduling:
  - Criticality
  - Flexible flag
  - EstimatedDuration
  - Deadline
- new node metadata:
  - CarbonIntensity
- an energy-aware scoring function in [pkg/scheduler/energy.go](pkg/scheduler/energy.go)
- integration of that score into the existing scheduler pipeline in [pkg/scheduler/pipeline.go](pkg/scheduler/pipeline.go)
- a regression-style test file at [pkg/scheduler/energy_test.go](pkg/scheduler/energy_test.go)
- a small fix in [pkg/worker/handler.go](pkg/worker/handler.go) where a non-constant log.Printf argument was being used incorrectly

The scheduler now follows a practical fusion strategy:

- keep the existing network-aware score
- add a secondary energy-aware cost term
- weight energy more heavily for flexible workloads
- reduce energy emphasis for critical workloads

This is a reasonable first step toward a combined network + energy scheduler without a full rewrite.

## 3. Why this is the feasible path

There are two realistic routes:

### Option A: Extend the existing custom orchestrator
This is the most feasible route because the repo already has:

- scheduler selection logic
- node scoring windows
- task placement flow
- manager-driven scheduling decisions

This path requires only incremental extension and is suitable for a lightweight prototype and a report.

### Option B: Re-implement as a Kubernetes scheduling plugin
This is a longer-term direction but is not the current state of this repo. It would require:

- Kubernetes scheduler framework integration
- plugin registration
- node filtering/scoring APIs
- cluster/integration testing
- deployment in a real k8s environment

This is a major migration and is not the fastest way to hand the current repo an energy-aware scheduler.

## 4. Key research framing

The strongest combined research story is:

- Diktyo supplies network-aware placement logic
- Saad et al. (2025) supplies the energy prediction signal
- Piontek et al. supplies the scheduling mechanism pattern for letting flexible jobs move to lower-carbon windows

The project contribution becomes: use the better energy prediction signal as the cost input for a scheduler that also respects network topology and latency constraints.

This is a clear, defensible academic framing.

## 5. Future work required

The current repo has a minimum viable extension, but several things are still needed for a stronger implementation:

### 5.1 Real energy model
Add a real energy estimator based on one of these sources:

- Kepler readouts
- CPU utilization-based model
- power estimation library
- historical energy traces

### 5.2 Real carbon intensity source
Use a carbon intensity feed or region-specific static factor for conversion from energy to CO2.

### 5.3 Better task scheduling policy
Define explicit rules for:

- critical vs flexible jobs
- deadlines and urgency
- delay windows for shiftable workloads
- fallback behavior when the carbon model is unavailable

### 5.4 Metrics and evaluation harness
Add a comparison harness for:

- default scheduler
- network-only scheduler
- energy-only scheduler
- fused network + energy scheduler

Track:

- CO2 emitted
- latency
- throughput
- deadline SLA violations
- scheduler overhead

### 5.5 K8s plugin porting (future long-term stretch)
If the goal is to align with Kubernetes research practice, port the same logic into a scheduler plugin once the design is mature.

## 6. Verification status

The repo was verified with:

- `go test ./...`

Result: pass, with the scheduler tests succeeding and the project compiling cleanly.

## 7. Conclusion

The repo is currently in a realistic middle ground:

- it already has a custom network-aware scheduler prototype
- energy logic has now been added as a minimal extension
- the repo is not yet a Kubernetes plugin
- the best near-term path is to continue enhancing the custom scheduler and treat the K8s plugin port as a future follow-on project

This is the most feasible path to get meaningful technical progress and a strong project story without re-architecting the whole system.
