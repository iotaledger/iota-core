package prometheus

import (
	"github.com/iotaledger/iota-core/components/prometheus/collector"
)

const (
	dbNamespace = "db"

	sizeBytesPermanent   = "size_bytes_permanent"
	sizeBytesPrunable    = "size_bytes_prunable"
	sizeBytesPrunableSQL = "size_bytes_prunable_SQL"
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
	collector.WithMetric(collector.NewMetric(sizeBytesPrunableSQL,
		collector.WithType(collector.Gauge),
		collector.WithHelp("DB size in bytes for prunable SQL storage."),
		collector.WithCollectFunc(func() (metricValue float64, labelValues []string) {
			return float64(deps.Protocol.Engines.Main.Get().Storage.PrunableSQLDatabaseSize()), nil
		}),
	)),
)
