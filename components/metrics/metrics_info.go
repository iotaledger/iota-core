package metrics

import (
	"runtime"
	"strconv"

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
		collector.WithType(collector.GaugeVec),
		collector.WithHelp("Node OS."),
		collector.WithLabels("nodeID", "OS", "ARCH", "NUM_CPU"),
		collector.WithLabelValuesCollection(), // when updating we set new label values otherwise we get inconsistent cardinality panic
		collector.WithInitValue(func() map[string]float64 {
			var nodeID string
			if deps.Local != nil {
				nodeID = deps.Local.ID().String()
			}
			return collector.MultiLabels(nodeID, runtime.GOOS, runtime.GOARCH, strconv.Itoa(runtime.GOMAXPROCS(0)))
		}),
	)),
	collector.WithMetric(collector.NewMetric(syncStatus,
		collector.WithType(collector.Gauge),
		collector.WithHelp("Node sync status based on TangleTime."),
		collector.WithCollectFunc(func() map[string]float64 {
			if deps.Protocol.MainEngineInstance().IsSynced() {
				return collector.SingleValue(1)
			}
			return collector.SingleValue(0)
		}),
	)),
	collector.WithMetric(collector.NewMetric(memUsage,
		collector.WithType(collector.Gauge),
		collector.WithHelp("The memory usage in bytes of allocated heap objects"),
		collector.WithCollectFunc(func() map[string]float64 {
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			return collector.SingleValue(float64(m.Alloc))
		}),
	)),
)
