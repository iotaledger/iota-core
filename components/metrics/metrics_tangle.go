package metrics

import (
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/iota-core/components/metrics/collector"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
)

const (
	tangleNamespace = "tangle"

	tipsCount                     = "tips_count"
	strongTipsCount               = "strong_tips_count"
	weakTipsCount                 = "weak_tips_count"
	blockPerTypeCount             = "block_per_type_total"
	blocksTotal                   = "blocks_total"
	missingBlocksCount            = "missing_block_total"
	parentPerTypeCount            = "parent_per_type_total"
	blocksPerComponentCount       = "blocks_per_component_total"
	timeSinceReceivedPerComponent = "time_since_received_per_component_seconds"
	requestQueueSize              = "request_queue_size"
	blocksOrphanedCount           = "blocks_orphaned_total"
	acceptedBlocksCount           = "accepted_blocks_count"
)

var TangleMetrics = collector.NewCollection(tangleNamespace,
	collector.WithMetric(collector.NewMetric(tipsCount,
		collector.WithType(collector.Gauge),
		collector.WithHelp("Number of tips in the tangle"),
		collector.WithCollectFunc(func() (metricValue float64, labelValues []string) {
			count := len(deps.Protocol.MainEngineInstance().TipManager.StrongTips()) + len(deps.Protocol.MainEngineInstance().TipManager.WeakTips())
			return float64(count), nil
		}),
	)),
	collector.WithMetric(collector.NewMetric(strongTipsCount,
		collector.WithType(collector.Gauge),
		collector.WithHelp("Number of tips in the tangle"),
		collector.WithCollectFunc(func() (metricValue float64, labelValues []string) {
			count := len(deps.Protocol.MainEngineInstance().TipManager.StrongTips())
			return float64(count), nil
		}),
	)),
	collector.WithMetric(collector.NewMetric(weakTipsCount,
		collector.WithType(collector.Gauge),
		collector.WithHelp("Number of tips in the tangle"),
		collector.WithCollectFunc(func() (metricValue float64, labelValues []string) {
			count := len(deps.Protocol.MainEngineInstance().TipManager.WeakTips())
			return float64(count), nil
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
	collector.WithMetric(collector.NewMetric(blocksTotal,
		collector.WithType(collector.Counter),
		collector.WithHelp("Total number of blocks attached."),
		collector.WithInitFunc(func() {
			deps.Protocol.Events.Engine.BlockDAG.BlockAttached.Hook(func(_ *blocks.Block) {
				deps.Collector.Increment(tangleNamespace, blocksTotal)
			}, event.WithWorkerPool(Component.WorkerPool))
		}),
	)),
	// collector.WithMetric(collector.NewMetric(blocksOrphanedCount,
	// 	collector.WithType(collector.Counter),
	// 	collector.WithHelp("Number of orphaned blocks"),
	// 	collector.WithInitFunc(func() {
	// 		deps.Protocol.Events.Engine.Tangle.BlockDAG.BlockOrphaned.Hook(func(block *blockdag.Block) {
	// 			deps.Collector.Increment(tangleNamespace, blocksOrphanedCount)
	// 		}, event.WithWorkerPool(Component.WorkerPool))
	// 	}),
	// )),
	collector.WithMetric(collector.NewMetric(acceptedBlocksCount,
		collector.WithType(collector.Counter),
		collector.WithHelp("Number of accepted blocks"),
		collector.WithInitFunc(func() {
			deps.Protocol.Events.Engine.BlockGadget.BlockAccepted.Hook(func(_ *blocks.Block) {
				deps.Collector.Increment(tangleNamespace, acceptedBlocksCount)
			}, event.WithWorkerPool(Component.WorkerPool))
		}),
	)),
	// collector.WithMetric(collector.NewMetric(timeSinceReceivedPerComponent,
	// 	collector.WithType(collector.Counter),
	// 	collector.WithHelp("Time since the block was received per component"),
	// 	collector.WithLabels("component"),
	// 	collector.WithInitFunc(func() {
	// 		deps.Protocol.Events.Engine.BlockGadget.BlockAccepted.Hook(func(block *blocks.Block) {
	// 			timeSince := float64(time.Since(block.IssuingTime()).Milliseconds())
	// 			deps.Collector.Update(tangleNamespace, timeSinceReceivedPerComponent, collector.MultiLabelsValues([]string{"block"}, timeSince))
	// 		}, event.WithWorkerPool(Component.WorkerPool))
	// 	}),
	// )),
)

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////
