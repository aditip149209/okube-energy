package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/aditip149209/okube/pkg/power"
	"github.com/aditip149209/okube/pkg/sim"
	"github.com/spf13/cobra"
)

var simulateCmd = &cobra.Command{
	Use:   "simulate",
	Short: "Compare the default, network-only, energy-only and fused schedulers offline.",
	Long: `Run the scheduler pipeline against a synthetic cluster and workload trace,
without a manager, workers or etcd.

Every workload is run over several seeds under the four configurations
(default, network-only, energy-only, fused), plus an energy-weight sweep for
energy-only and fused. Within a seed all configurations see the identical
trace, so the reported "vs default" changes are paired.

Results are written as CSV files to --out and a summary is printed:
  runs.csv      one row per simulation
  summary.csv   the four configurations, mean and std over seeds
  sweep.csv     energy-only and fused at each swept energy weight`,
	Run: func(cmd *cobra.Command, args []string) {
		outDir, _ := cmd.Flags().GetString("out")
		seeds, _ := cmd.Flags().GetInt("seeds")
		workloadNames, _ := cmd.Flags().GetStringSlice("workloads")
		sweep, _ := cmd.Flags().GetFloat64Slice("sweep")
		curvesPath, _ := cmd.Flags().GetString("power-curves")

		scenario := sim.DefaultScenario()
		if curvesPath != "" {
			curves, err := power.Load(curvesPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			curve, ok := curves[sim.DefaultNodeType]
			if !ok {
				fmt.Fprintf(os.Stderr, "Error: %s has no node type %q\n", curvesPath, sim.DefaultNodeType)
				os.Exit(1)
			}
			scenario = sim.BuildDefaultScenario(curve)
		}

		available := map[string]sim.TraceParams{
			"default": sim.DefaultTraceParams(),
			"stress":  sim.StressTraceParams(),
		}
		opts := sim.ExperimentOptions{Seeds: seeds, SweepWeights: sweep}
		for _, name := range workloadNames {
			params, ok := available[name]
			if !ok {
				fmt.Fprintf(os.Stderr, "Error: unknown workload %q (choose default and/or stress)\n", name)
				os.Exit(1)
			}
			opts.Workloads = append(opts.Workloads, sim.Workload{Name: name, Params: params})
		}

		rows, err := sim.RunExperiment(scenario, opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		agg := sim.Aggregate(rows)

		if err := os.MkdirAll(outDir, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating %s: %v\n", outDir, err)
			os.Exit(1)
		}
		outputs := []struct {
			name  string
			write func(f *os.File) error
		}{
			{"runs.csv", func(f *os.File) error { return sim.WriteRunsCSV(f, rows) }},
			{"summary.csv", func(f *os.File) error { return sim.WriteSummaryCSV(f, agg) }},
			{"sweep.csv", func(f *os.File) error { return sim.WriteSweepCSV(f, agg) }},
		}
		for _, o := range outputs {
			path := filepath.Join(outDir, o.name)
			f, err := os.Create(path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error creating %s: %v\n", path, err)
				os.Exit(1)
			}
			werr := o.write(f)
			cerr := f.Close()
			if werr != nil || cerr != nil {
				fmt.Fprintf(os.Stderr, "Error writing %s: %v %v\n", path, werr, cerr)
				os.Exit(1)
			}
		}

		fmt.Print(sim.FormatTables(agg))
		fmt.Printf("Wrote runs.csv, summary.csv and sweep.csv to %s\n", outDir)
	},
}

func init() {
	rootCmd.AddCommand(simulateCmd)

	simulateCmd.Flags().String("out", "sim-results", "Directory to write the CSV results to")
	simulateCmd.Flags().Int("seeds", 20, "Number of seeds (independent traces) per workload")
	simulateCmd.Flags().StringSlice("workloads", []string{"default", "stress"}, "Workloads to run: default (~35% load) and/or stress (saturating)")
	simulateCmd.Flags().String("power-curves", "", "power_curves.json from models/train_power_model.py (default: curves embedded in the binary)")
	simulateCmd.Flags().Float64Slice("sweep", []float64{0, 0.1, 0.2, 0.5, 1, 2, 5}, "Energy weights for the energy-only and fused sweep")
}
