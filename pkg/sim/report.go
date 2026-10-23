package sim

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"
)

func f(v float64) string { return strconv.FormatFloat(v, 'f', 4, 64) }

// WriteRunsCSV writes one row per simulation (workload, config, weight, seed).
func WriteRunsCSV(w io.Writer, rows []RunRow) error {
	cw := csv.NewWriter(w)
	header := []string{
		"workload", "config", "energy_weight", "seed", "tasks", "makespan_sec",
		"dynamic_energy_kwh", "dynamic_co2_g", "idle_co2_g", "total_co2_g",
		"mean_latency_ms", "p95_latency_ms",
		"deadline_violations", "deadline_violation_rate", "mean_queue_wait_sec",
		"mean_decision_us", "p95_decision_us",
		"flexible_intensity_g_kwh", "critical_intensity_g_kwh", "default_intensity_g_kwh",
	}
	if err := cw.Write(header); err != nil {
		return err
	}
	for _, r := range rows {
		s := r.Summary
		rec := []string{
			r.Workload, r.Config, r.EnergyWeight, strconv.FormatInt(r.Seed, 10),
			strconv.Itoa(s.Tasks), f(s.MakespanSec),
			f(s.DynamicEnergyKWh), f(s.DynamicCO2G), f(s.IdleCO2G), f(s.TotalCO2G),
			f(s.MeanLatencyMS), f(s.P95LatencyMS),
			strconv.Itoa(s.DeadlineViolations), f(s.DeadlineViolationRate), f(s.MeanQueueWaitSec),
			f(s.MeanDecisionUS), f(s.P95DecisionUS),
			f(s.ByClass[ClassFlexible].MeanIntensityGKWh),
			f(s.ByClass[ClassCritical].MeanIntensityGKWh),
			f(s.ByClass[ClassDefault].MeanIntensityGKWh),
		}
		if err := cw.Write(rec); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

var aggHeader = []string{
	"workload", "config", "energy_weight", "runs",
	"dynamic_co2_g_mean", "dynamic_co2_g_std", "total_co2_g_mean", "total_co2_g_std",
	"co2_vs_default_pct_mean", "co2_vs_default_pct_std",
	"mean_latency_ms_mean", "mean_latency_ms_std", "p95_latency_ms_mean", "p95_latency_ms_std",
	"latency_vs_default_pct_mean", "latency_vs_default_pct_std",
	"violation_rate_mean", "violation_rate_std",
	"queue_wait_sec_mean", "queue_wait_sec_std",
	"decision_us_mean", "decision_us_std",
	"flexible_intensity_mean", "critical_intensity_mean",
}

func aggRecord(a AggRow) []string {
	return []string{
		a.Workload, a.Config, a.EnergyWeight, strconv.Itoa(a.Runs),
		f(a.DynamicCO2G.Mean), f(a.DynamicCO2G.Std), f(a.TotalCO2G.Mean), f(a.TotalCO2G.Std),
		f(a.CO2VsDefaultPct.Mean), f(a.CO2VsDefaultPct.Std),
		f(a.MeanLatencyMS.Mean), f(a.MeanLatencyMS.Std), f(a.P95LatencyMS.Mean), f(a.P95LatencyMS.Std),
		f(a.LatencyVsDefaultPct.Mean), f(a.LatencyVsDefaultPct.Std),
		f(a.ViolationRate.Mean), f(a.ViolationRate.Std),
		f(a.MeanQueueWaitSec.Mean), f(a.MeanQueueWaitSec.Std),
		f(a.MeanDecisionUS.Mean), f(a.MeanDecisionUS.Std),
		f(a.FlexibleIntensity.Mean), f(a.CriticalIntensity.Mean),
	}
}

func writeAggCSV(w io.Writer, rows []AggRow) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(aggHeader); err != nil {
		return err
	}
	for _, a := range rows {
		if err := cw.Write(aggRecord(a)); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// isSweepRow reports whether an aggregate row is a point of the energy-weight
// sweep, as opposed to one of the four default-weight configs.
func isSweepRow(a AggRow) bool { return a.EnergyWeight != WeightDefault }

// WriteSummaryCSV writes the four-config comparison at default weights.
func WriteSummaryCSV(w io.Writer, agg []AggRow) error {
	return writeAggCSV(w, filterAgg(agg, func(a AggRow) bool { return !isSweepRow(a) }))
}

// WriteSweepCSV writes the energy-weight sweep for energy-only and fused.
func WriteSweepCSV(w io.Writer, agg []AggRow) error {
	return writeAggCSV(w, filterAgg(agg, isSweepRow))
}

func filterAgg(agg []AggRow, keep func(AggRow) bool) []AggRow {
	var out []AggRow
	for _, a := range agg {
		if keep(a) {
			out = append(out, a)
		}
	}
	return out
}

func pm(s Stat, prec int) string {
	return fmt.Sprintf("%.*f±%.*f", prec, s.Mean, prec, s.Std)
}

func pct(s Stat) string { return fmt.Sprintf("%+.1f%%", s.Mean) }

// FormatTables renders the summary and the sweep as aligned text for a
// terminal, one block per workload.
func FormatTables(agg []AggRow) string {
	var workloads []string
	seen := map[string]bool{}
	for _, a := range agg {
		if !seen[a.Workload] {
			seen[a.Workload] = true
			workloads = append(workloads, a.Workload)
		}
	}

	var b strings.Builder
	for _, wl := range workloads {
		runs := 0
		for _, a := range agg {
			if a.Workload == wl {
				runs = a.Runs
				break
			}
		}
		fmt.Fprintf(&b, "== workload %q: mean±std over %d seeds; vs-default is paired per seed ==\n", wl, runs)

		tw := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "config\tdyn CO2 (g)\tvs default\tmean lat (ms)\tp95 lat (ms)\tlat vs default\tviol rate\tqueue wait (s)\tdecision (µs)")
		for _, a := range agg {
			if a.Workload != wl || isSweepRow(a) {
				continue
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				a.Config, pm(a.DynamicCO2G, 2), pct(a.CO2VsDefaultPct),
				pm(a.MeanLatencyMS, 2), pm(a.P95LatencyMS, 1), pct(a.LatencyVsDefaultPct),
				pm(a.ViolationRate, 3), pm(a.MeanQueueWaitSec, 1), pm(a.MeanDecisionUS, 1))
		}
		tw.Flush()

		fmt.Fprintf(&b, "\n-- energy-weight sweep, workload %q --\n", wl)
		tw = tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "config\tw_energy\tdyn CO2 (g)\tvs default\tmean lat (ms)\tlat vs default\tflexible g/kWh\tcritical g/kWh")
		for _, a := range agg {
			if a.Workload != wl || !isSweepRow(a) {
				continue
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%.0f\t%.0f\n",
				a.Config, a.EnergyWeight, pm(a.DynamicCO2G, 2), pct(a.CO2VsDefaultPct),
				pm(a.MeanLatencyMS, 2), pct(a.LatencyVsDefaultPct),
				a.FlexibleIntensity.Mean, a.CriticalIntensity.Mean)
		}
		tw.Flush()
		b.WriteString("\n")
	}
	return b.String()
}
