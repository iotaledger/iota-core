package prometheus

import (
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/iota-core/components/prometheus/collector"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
)

const (
	tangleNamespace = "tangle"

	strongTipsCount     = "strong_tips_count"
	weakTipsCount       = "weak_tips_count"
	bookedBlocksTotal   = "booked_blocks_total"
	missingBlocksCount  = "missing_blocks_total"
	acceptedBlocksCount = "accepted_blocks_count"
)

var TangleMetrics = collector.NewCollection(tangleNamespace,
	collector.WithMetric(collector.NewMetric(strongTipsCount,
		collector.WithType(collector.Gauge),
		collector.WithHelp("Number of strong tips in the tangle"),
		collector.WithCollectFunc(func() (metricValue float64, labelValues []string) {
			count := len(deps.Protocol.Engines.Main.Get().TipManager.StrongTips())

			return float64(count), nil
		}),
	)),
	collector.WithMetric(collector.NewMetric(weakTipsCount,
		collector.WithType(collector.Gauge),
		collector.WithHelp("Number of weak tips in the tangle"),
		collector.WithCollectFunc(func() (metricValue float64, labelValues []string) {
			count := len(deps.Protocol.Engines.Main.Get().TipManager.WeakTips())

			return float64(count), nil
		}),
	)),
	collector.WithMetric(collector.NewMetric(bookedBlocksTotal,
		collector.WithType(collector.Counter),
		collector.WithHelp("Total number of blocks booked."),
		collector.WithInitFunc(func() {
			deps.Protocol.Events.Engine.Booker.BlockBooked.Hook(func(_ *blocks.Block) {
				deps.Collector.Increment(tangleNamespace, bookedBlocksTotal)
			}, event.WithWorkerPool(Component.WorkerPool))
		}),
	)),
	collector.WithMetric(collector.NewMetric(missingBlocksCount,
		collector.WithType(collector.Counter),
		collector.WithHelp("Number of blocks missing during the solidification in the tangle"),
		collector.WithInitFunc(func() {
			deps.Protocol.Events.Engine.BlockDAG.BlockMissing.Hook(func(_ *blocks.Block) {
				deps.Collector.Increment(tangleNamespace, missingBlocksCount)
			}, event.WithWorkerPool(Component.WorkerPool))
		}),
	)),
	collector.WithMetric(collector.NewMetric(acceptedBlocksCount,
		collector.WithType(collector.Counter),
		collector.WithHelp("Number of accepted blocks"),
		collector.WithInitFunc(func() {
			deps.Protocol.Events.Engine.BlockGadget.BlockAccepted.Hook(func(_ *blocks.Block) {
				deps.Collector.Increment(tangleNamespace, acceptedBlocksCount)
			}, event.WithWorkerPool(Component.WorkerPool))
		}),
	)),
)
