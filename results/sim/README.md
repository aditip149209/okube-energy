# Simulator results

Produced by `okube simulate` (see `pkg/sim`). CO2 is **dynamic** CO2 per run in
grams (power above idle, integrated over time from the SPEC-trained curves in
`pkg/power`), so it is a relative comparison between schedulers, not real-world
emissions. Carbon intensities are illustrative UK-like values, not measured.

## Reproduce

```
go run . simulate --out results/sim --seeds 50 --first-seed 101 \
  --sweep 0,0.1,0.2,0.5,1,1.5,2,3,5,8,10
```

- `runs.csv`: one row per simulation (workload, config, energy weight, seed).
- `summary.csv`: the four configurations at the scheduler's default energy
  weight (0.2). Mean and std over seeds; "vs default" is paired per seed.
- `sweep.csv`: energy-only and fused at each swept energy weight.

## Choosing the headline energy weight

The weight was selected on seeds 1-30 and the results above come from the
disjoint seeds 101-150, so the reported numbers were not used to choose it.

Rule: the largest energy weight whose mean latency stays within 10% of
network-only and whose deadline-violation rate does not rise, on both the
`default` and `stress` workloads. That gives **w = 1.5** (latency +6.6% and
+7.7% vs network-only on seeds 1-30; w = 2 exceeds the budget on `stress`).
The 10% budget is a judgement call: a 25% budget would select w = 3.

## Headline (seeds 101-150, 50 seeds)

| workload | config | w | CO2 vs default | mean latency vs default |
|---|---|---|---|---|
| default | network-only | - | -1.7% | -83.3% |
| default | energy-only | 0.2 | -2.8% | -12.9% |
| default | fused | 0.2 | -5.2% | -84.6% |
| default | fused | 1.5 | -12.9% | -81.0% |
| stress | fused | 0.2 | -1.1% | -57.2% |
| stress | fused | 1.5 | -2.4% | -53.7% |

Caveats: single learned power model (not federated); the scheduler's decisions
do not use the model (it only affects CO2 accounting); the `stress` workload
saturates the cluster so placement freedom is scarce; deadline violations do
not differ between configs.
