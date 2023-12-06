package metrics

import (
	"github.com/iotaledger/iota-core/components/metrics/collector"
)

const (
	dbNamespace = "db"

	sizeBytesPermanent = "size_bytes_permanent"
	sizeBytesPrunable  = "size_bytes_prunable"
	sizeBytesRetainer  = "size_bytes_retainer"
)

var DBMetrics = collector.NewCollection(dbNamespace,
	collector.WithMetric(collector.NewMetric(sizeBytesPermanent,
		collector.WithType(collector.Gauge),
		collector.WithHelp("DB size in bytes for permanent storage."),
		collector.WithCollectFunc(func() (metricValue float64, labelValues []string) {
			return float64(deps.Protocol.Engines.Main.Get().Storage.PermanentDatabaseSize()), nil
		}),
	)),
	collector.WithMetric(collector.NewMetric(sizeBytesPrunable,
		collector.WithType(collector.Gauge),
		collector.WithHelp("DB size in bytes for prunable storage."),
		collector.WithCollectFunc(func() (metricValue float64, labelValues []string) {
			return float64(deps.Protocol.Engines.Main.Get().Storage.PrunableDatabaseSize()), nil
		}),
	)),
)
