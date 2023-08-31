package metrics

import (
	"runtime"
	"strconv"
	"time"

	"github.com/iotaledger/iota-core/components/metrics/collector"
)

const (
	infoNamespace = "info"

	appName    = "app"
	nodeOS     = "node_os"
	syncStatus = "sync_status"
	memUsage   = "memory_usage_bytes"
)

var InfoMetrics = collector.NewCollection(infoNamespace,
	collector.WithMetric(collector.NewMetric(nodeOS,
		collector.WithType(collector.Gauge),
		collector.WithHelp("Node OS data."),
		collector.WithLabels("nodeID", "OS", "ARCH", "NUM_CPU"),
		collector.WithPruningDelay(10*time.Minute),
		collector.WithInitValueFunc(func() (metricValue float64, labelValues []string) {
			var nodeID string
			if deps.Host != nil {
				nodeID = deps.Host.ID().String()
			}

			return 0, []string{nodeID, runtime.GOOS, runtime.GOARCH, strconv.Itoa(runtime.NumCPU())}
		}),
	)),
	collector.WithMetric(collector.NewMetric(syncStatus,
		collector.WithType(collector.Gauge),
		collector.WithHelp("Node sync status based on ATT."),
		collector.WithCollectFunc(func() (metricValue float64, labelValues []string) {
			if deps.Protocol.MainEngineInstance().SyncManager.IsNodeSynced() {
				return 1, nil
			}

			return 0, nil
		}),
	)),
	collector.WithMetric(collector.NewMetric(memUsage,
		collector.WithType(collector.Gauge),
		collector.WithHelp("The memory usage in bytes of allocated heap objects"),
		collector.WithCollectFunc(func() (metricValue float64, labelValues []string) {
			var m runtime.MemStats
			runtime.ReadMemStats(&m)

			return float64(m.Alloc), nil
		}),
	)),
)
