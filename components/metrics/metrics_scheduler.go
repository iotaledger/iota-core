//nolint:gosec // false positive on constants
package metrics

import (
	"time"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/iota-core/components/metrics/collector"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
)

const (
	schedulerNamespace = "scheduler"

	queueSizePerNodeWork           = "queue_size_per_node_work" //nolint:gosec
	queueSizePerNodeCount          = "queue_size_per_node_count"
	validatorQueueSizePerNodeCount = "validator_queue_size_per_node_count"
	schedulerProcessedBlocks       = "processed_blocks"
	manaAmountPerNode              = "mana_per_node"
	scheduledBlockLabel            = "scheduled"
	skippedBlockLabel              = "skipped"
	droppedBlockLabel              = "dropped"
	enqueuedBlockLabel             = "enqueued"
	basicBufferReadyBlockCount     = "buffer_ready_block_total" //nolint:gosec
	basicBufferTotalSize           = "buffer_size_block_total"
	basicBufferMaxSize             = "buffer_max_size"
	rate                           = "rate"
	validatorBufferTotalSize       = "validator_buffer_size_block_total"
	validatorQueueMaxSize          = "validator_buffer_max_size"
)

var SchedulerMetrics = collector.NewCollection(schedulerNamespace,
	collector.WithMetric(collector.NewMetric(queueSizePerNodeWork,
		collector.WithType(collector.Gauge),
		collector.WithLabels("issuer_id"),
		collector.WithPruningDelay(10*time.Minute),
		collector.WithHelp("Current size of each node's queue (in work units)."),
		collector.WithInitFunc(func() {
			deps.Protocol.Events.Engine.Scheduler.BlockEnqueued.Hook(func(block *blocks.Block) {
				deps.Collector.Update(schedulerNamespace, queueSizePerNodeWork, float64(deps.Protocol.MainEngineInstance().Scheduler.IssuerQueueWork(block.ProtocolBlock().IssuerID)), block.ProtocolBlock().IssuerID.String())

			}, event.WithWorkerPool(Component.WorkerPool))

			deps.Protocol.Events.Engine.Scheduler.BlockSkipped.Hook(func(block *blocks.Block) {
				deps.Collector.Update(schedulerNamespace, queueSizePerNodeWork, float64(deps.Protocol.MainEngineInstance().Scheduler.IssuerQueueWork(block.ProtocolBlock().IssuerID)), block.ProtocolBlock().IssuerID.String())

			}, event.WithWorkerPool(Component.WorkerPool))

			deps.Protocol.Events.Engine.Scheduler.BlockDropped.Hook(func(block *blocks.Block, _ error) {
				deps.Collector.Update(schedulerNamespace, queueSizePerNodeWork, float64(deps.Protocol.MainEngineInstance().Scheduler.IssuerQueueWork(block.ProtocolBlock().IssuerID)), block.ProtocolBlock().IssuerID.String())

			}, event.WithWorkerPool(Component.WorkerPool))

			deps.Protocol.Events.Engine.Scheduler.BlockScheduled.Hook(func(block *blocks.Block) {
				deps.Collector.Update(schedulerNamespace, queueSizePerNodeWork, float64(deps.Protocol.MainEngineInstance().Scheduler.IssuerQueueWork(block.ProtocolBlock().IssuerID)), block.ProtocolBlock().IssuerID.String())

			}, event.WithWorkerPool(Component.WorkerPool))
		}),
	)),
	collector.WithMetric(collector.NewMetric(queueSizePerNodeCount,
		collector.WithType(collector.Gauge),
		collector.WithLabels("issuer_id"),
		collector.WithPruningDelay(10*time.Minute),
		collector.WithHelp("Current size of each node's queue (as block count)."),
		collector.WithInitFunc(func() {
			deps.Protocol.Events.Engine.Scheduler.BlockEnqueued.Hook(func(block *blocks.Block) {
				if _, isBasic := block.BasicBlock(); isBasic {
					deps.Collector.Update(schedulerNamespace, queueSizePerNodeCount, float64(deps.Protocol.MainEngineInstance().Scheduler.IssuerQueueBlockCount(block.ProtocolBlock().IssuerID)), block.ProtocolBlock().IssuerID.String())
				}
			}, event.WithWorkerPool(Component.WorkerPool))

			deps.Protocol.Events.Engine.Scheduler.BlockSkipped.Hook(func(block *blocks.Block) {
				if _, isBasic := block.BasicBlock(); isBasic {
					deps.Collector.Update(schedulerNamespace, queueSizePerNodeCount, float64(deps.Protocol.MainEngineInstance().Scheduler.IssuerQueueBlockCount(block.ProtocolBlock().IssuerID)), block.ProtocolBlock().IssuerID.String())
				}
			}, event.WithWorkerPool(Component.WorkerPool))

			deps.Protocol.Events.Engine.Scheduler.BlockDropped.Hook(func(block *blocks.Block, _ error) {
				if _, isBasic := block.BasicBlock(); isBasic {
					deps.Collector.Update(schedulerNamespace, queueSizePerNodeCount, float64(deps.Protocol.MainEngineInstance().Scheduler.IssuerQueueBlockCount(block.ProtocolBlock().IssuerID)), block.ProtocolBlock().IssuerID.String())
				}
			}, event.WithWorkerPool(Component.WorkerPool))

			deps.Protocol.Events.Engine.Scheduler.BlockScheduled.Hook(func(block *blocks.Block) {
				if _, isBasic := block.BasicBlock(); isBasic {
					deps.Collector.Update(schedulerNamespace, queueSizePerNodeCount, float64(deps.Protocol.MainEngineInstance().Scheduler.IssuerQueueBlockCount(block.ProtocolBlock().IssuerID)), block.ProtocolBlock().IssuerID.String())
				}
			}, event.WithWorkerPool(Component.WorkerPool))
		}),
	)),
	collector.WithMetric(collector.NewMetric(validatorQueueSizePerNodeCount,
		collector.WithType(collector.Gauge),
		collector.WithLabels("issuer_id"),
		collector.WithPruningDelay(10*time.Minute),
		collector.WithHelp("Current number of validation blocks in each validator's queue."),
		collector.WithInitFunc(func() {
			deps.Protocol.Events.Engine.Scheduler.BlockEnqueued.Hook(func(block *blocks.Block) {
				if _, isValidation := block.ValidationBlock(); isValidation {
					deps.Collector.Update(schedulerNamespace, validatorQueueSizePerNodeCount, float64(deps.Protocol.MainEngineInstance().Scheduler.ValidatorQueueBlockCount(block.ProtocolBlock().IssuerID)), block.ProtocolBlock().IssuerID.String())
				}
			}, event.WithWorkerPool(Component.WorkerPool))

			deps.Protocol.Events.Engine.Scheduler.BlockSkipped.Hook(func(block *blocks.Block) {
				if _, isValidation := block.ValidationBlock(); isValidation {
					deps.Collector.Update(schedulerNamespace, validatorQueueSizePerNodeCount, float64(deps.Protocol.MainEngineInstance().Scheduler.ValidatorQueueBlockCount(block.ProtocolBlock().IssuerID)), block.ProtocolBlock().IssuerID.String())
				}
			}, event.WithWorkerPool(Component.WorkerPool))

			deps.Protocol.Events.Engine.Scheduler.BlockDropped.Hook(func(block *blocks.Block, _ error) {
				if _, isValidation := block.ValidationBlock(); isValidation {
					deps.Collector.Update(schedulerNamespace, validatorQueueSizePerNodeCount, float64(deps.Protocol.MainEngineInstance().Scheduler.ValidatorQueueBlockCount(block.ProtocolBlock().IssuerID)), block.ProtocolBlock().IssuerID.String())
				}
			}, event.WithWorkerPool(Component.WorkerPool))

			deps.Protocol.Events.Engine.Scheduler.BlockScheduled.Hook(func(block *blocks.Block) {
				if _, isValidation := block.ValidationBlock(); isValidation {
					deps.Collector.Update(schedulerNamespace, validatorQueueSizePerNodeCount, float64(deps.Protocol.MainEngineInstance().Scheduler.ValidatorQueueBlockCount(block.ProtocolBlock().IssuerID)), block.ProtocolBlock().IssuerID.String())
				}
			}, event.WithWorkerPool(Component.WorkerPool))
		}),
	)),
	collector.WithMetric(collector.NewMetric(manaAmountPerNode,
		collector.WithType(collector.Gauge),
		collector.WithLabels("issuer_id"),
		collector.WithPruningDelay(10*time.Minute),
		collector.WithHelp("Current amount of mana of each issuer in the queue."),
		collector.WithInitFunc(func() {
			deps.Protocol.Events.Engine.Scheduler.BlockEnqueued.Hook(func(block *blocks.Block) {
				mana, err := deps.Protocol.MainEngineInstance().Ledger.ManaManager().GetManaOnAccount(block.ProtocolBlock().IssuerID, block.SlotCommitmentID().Index())
				if err != nil {
					deps.Protocol.MainEngineInstance().ErrorHandler("metrics")(ierrors.Wrapf(err, "failed to retrieve mana on account %s for slot %d", block.ProtocolBlock().IssuerID, block.SlotCommitmentID().Index()))

					return
				}

				deps.Collector.Update(schedulerNamespace, manaAmountPerNode, float64(mana), block.ProtocolBlock().IssuerID.String())
			}, event.WithWorkerPool(Component.WorkerPool))
		}),
	)),
	collector.WithMetric(collector.NewMetric(schedulerProcessedBlocks,
		collector.WithType(collector.Counter),
		collector.WithLabels("state"),
		collector.WithHelp("Number of blocks processed by the scheduler."),
		collector.WithInitFunc(func() {
			deps.Protocol.Events.Engine.Scheduler.BlockEnqueued.Hook(func(block *blocks.Block) {
				deps.Collector.Increment(schedulerNamespace, schedulerProcessedBlocks, enqueuedBlockLabel)

			}, event.WithWorkerPool(Component.WorkerPool))

			deps.Protocol.Events.Engine.Scheduler.BlockDropped.Hook(func(block *blocks.Block, _ error) {
				deps.Collector.Increment(schedulerNamespace, schedulerProcessedBlocks, droppedBlockLabel)

			}, event.WithWorkerPool(Component.WorkerPool))

			deps.Protocol.Events.Engine.Scheduler.BlockSkipped.Hook(func(block *blocks.Block) {
				deps.Collector.Increment(schedulerNamespace, schedulerProcessedBlocks, skippedBlockLabel)

			}, event.WithWorkerPool(Component.WorkerPool))

			deps.Protocol.Events.Engine.Scheduler.BlockScheduled.Hook(func(block *blocks.Block) {
				deps.Collector.Increment(schedulerNamespace, schedulerProcessedBlocks, scheduledBlockLabel)

			}, event.WithWorkerPool(Component.WorkerPool))
		}),
	)),
	collector.WithMetric(collector.NewMetric(basicBufferMaxSize,
		collector.WithType(collector.Gauge),
		collector.WithHelp("Maximum number of basic blocks that can be stored in the buffer."),
		collector.WithCollectFunc(func() (float64, []string) {
			return float64(deps.Protocol.MainEngineInstance().CurrentAPI().ProtocolParameters().CongestionControlParameters().MaxBufferSize), []string{}
		}),
	)),
	collector.WithMetric(collector.NewMetric(basicBufferReadyBlockCount,
		collector.WithType(collector.Gauge),
		collector.WithHelp("Number of ready blocks in the scheduler buffer."),
		collector.WithCollectFunc(func() (float64, []string) {
			return float64(deps.Protocol.MainEngineInstance().Scheduler.ReadyBlocksCount()), []string{}
		}),
	)),
	collector.WithMetric(collector.NewMetric(basicBufferTotalSize,
		collector.WithType(collector.Gauge),
		collector.WithHelp("Current number of basic blocks in the scheduler buffer."),
		collector.WithCollectFunc(func() (float64, []string) {
			return float64(deps.Protocol.MainEngineInstance().Scheduler.BasicBufferSize()), []string{}
		}),
	)),
	collector.WithMetric(collector.NewMetric(rate,
		collector.WithType(collector.Gauge),
		collector.WithHelp("Current scheduling rate of basic blocks."),
		collector.WithCollectFunc(func() (float64, []string) {
			return float64(deps.Protocol.MainEngineInstance().CurrentAPI().ProtocolParameters().CongestionControlParameters().SchedulerRate), []string{}
		}),
	)),
	collector.WithMetric(collector.NewMetric(validatorBufferTotalSize,
		collector.WithType(collector.Gauge),
		collector.WithHelp("Current number of validation blocks in the scheduling buffer."),
		collector.WithCollectFunc(func() (float64, []string) {
			return float64(deps.Protocol.MainEngineInstance().Scheduler.ValidatorBufferSize()), []string{}
		}),
	)),
	collector.WithMetric(collector.NewMetric(validatorQueueMaxSize,
		collector.WithType(collector.Gauge),
		collector.WithHelp("Maximum number of validation blocks that can be stored in each validator queue."),
		collector.WithCollectFunc(func() (float64, []string) {
			return float64(deps.Protocol.MainEngineInstance().CurrentAPI().ProtocolParameters().CongestionControlParameters().MaxValidationBufferSize), []string{}
		}),
	)),
)
