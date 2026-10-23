package cmd

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/aditip149209/okube/pkg/manager"
	"github.com/aditip149209/okube/pkg/store"
	"github.com/spf13/cobra"
)

var managerCmd = &cobra.Command{
	Use:   "manager",
	Short: "Start an okube manager node.",
	Long: `Start an okube manager that participates in leader election via etcd
and serves the manager HTTP API. Multiple managers can run simultaneously
for high availability.`,
	Run: func(cmd *cobra.Command, args []string) {
		host, _ := cmd.Flags().GetString("host")
		port, _ := cmd.Flags().GetInt("port")
		schedulerType, _ := cmd.Flags().GetString("scheduler")
		etcdEndpoints, _ := cmd.Flags().GetString("etcd-endpoints")
		workers, _ := cmd.Flags().GetString("workers")
		id, _ := cmd.Flags().GetString("id")
		queueSortStrategy, _ := cmd.Flags().GetString("queue-sort")
		topologyProbeMode, _ := cmd.Flags().GetString("topology-probe-mode")
		topologyProbeInterval, _ := cmd.Flags().GetDuration("topology-probe-interval")
		topologyProbeSampleSize, _ := cmd.Flags().GetInt("topology-probe-sample-size")
		networkWeight, _ := cmd.Flags().GetFloat64("network-weight")
		resourceWeight, _ := cmd.Flags().GetFloat64("resource-weight")
		energyWeight, _ := cmd.Flags().GetFloat64("energy-weight")
		disableNetworkScore, _ := cmd.Flags().GetBool("disable-network-score")
		disableEnergyScore, _ := cmd.Flags().GetBool("disable-energy-score")
		carbonIntensityFile, _ := cmd.Flags().GetString("carbon-intensity-file")

		endpoints := strings.Split(etcdEndpoints, ",")
		etcdStore, err := store.NewEtcdStore(store.EtcdConfig{
			Endpoints:   endpoints,
			DialTimeout: 5 * time.Second,
		})
		if err != nil {
			log.Fatalf("Failed to initialize etcd store: %v", err)
		}

		var workerList []string
		if workers != "" {
			for _, w := range strings.Split(workers, ",") {
				w = strings.TrimSpace(w)
				if w != "" {
					workerList = append(workerList, w)
				}
			}
		}

		advertiseAddr := fmt.Sprintf("%s:%d", host, port)
		m := manager.NewWithConfig(manager.Config{
			Workers:                 workerList,
			SchedulerType:           schedulerType,
			QueueSortStrategy:       queueSortStrategy,
			TopologyProbeMode:       topologyProbeMode,
			TopologyProbeInterval:   topologyProbeInterval,
			TopologyProbeSampleSize: topologyProbeSampleSize,
			Store:                   etcdStore,
			ID:                      id,
			AdvertiseAddr:           advertiseAddr,
			NetworkWeight:           &networkWeight,
			ResourceWeight:          &resourceWeight,
			EnergyWeight:            &energyWeight,
			DisableNetworkScore:     disableNetworkScore,
			DisableEnergyScore:      disableEnergyScore,
			CarbonIntensityFile:     carbonIntensityFile,
		})

		go m.UpdateTasks()
		go m.DoHealthChecks()

		mapi := manager.Api{Address: host, Port: port, Manager: m}
		log.Printf("Starting manager %s on http://%s", m.ID, advertiseAddr)
		mapi.Start()
	},
}

func init() {
	rootCmd.AddCommand(managerCmd)
	managerCmd.Flags().StringP("host", "H", "0.0.0.0", "Hostname or IP address to bind to")
	managerCmd.Flags().IntP("port", "p", 5556, "Port on which to listen")
	managerCmd.Flags().String("scheduler", "roundrobin", "Scheduler type (roundrobin, epvm or leastloaded)")
	managerCmd.Flags().String("queue-sort", "kahn", "Queue sort strategy (kahn, reversekahn, alternatekahn)")
	managerCmd.Flags().String("topology-probe-mode", "full-mesh", "Topology probe mode (full-mesh or sampled)")
	managerCmd.Flags().Duration("topology-probe-interval", 30*time.Second, "Interval between topology probe updates")
	managerCmd.Flags().Int("topology-probe-sample-size", 2, "Per-node sample count when topology probe mode is sampled")
	managerCmd.Flags().String("etcd-endpoints", "localhost:2379", "Comma-separated etcd endpoints")
	managerCmd.Flags().StringP("workers", "w", "", "Comma-separated initial worker addresses (host:port)")
	managerCmd.Flags().String("id", "", "Manager ID (defaults to hostname or random UUID)")
	managerCmd.Flags().Float64("network-weight", 0.7, "Weight applied to network cost in the fused score")
	managerCmd.Flags().Float64("resource-weight", 0.3, "Weight applied to resource cost in the fused score")
	managerCmd.Flags().Float64("energy-weight", 0.2, "Weight applied to energy/carbon cost in the fused score")
	managerCmd.Flags().Bool("disable-network-score", false, "Disable network-aware scoring (for baseline evaluation runs)")
	managerCmd.Flags().Bool("disable-energy-score", false, "Disable energy-aware scoring (for baseline evaluation runs)")
	managerCmd.Flags().String("carbon-intensity-file", "", "Path to a JSON file mapping worker ID/address to carbon intensity (see carbon_intensity.json); falls back to a fixed default when unset")
}
