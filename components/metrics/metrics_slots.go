package metrics

import (
	"strconv"
	"time"

	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/iota-core/components/metrics/collector"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	iotago "github.com/iotaledger/iota.go/v4"
)

const (
	slotNamespace             = "slots"
	slotLabelName             = "slot"
	metricEvictionOffset      = 6
	totalBlocks               = "total_blocks"
	acceptedBlocksInSlot      = "accepted_blocks"
	invalidBlocks             = "invalid_blocks"
	subjectivelyInvalidBlocks = "subjectively_invalid_blocks"
	acceptedAttachments       = "accepted_attachments"
	createdConflicts          = "created_conflicts"
	acceptedConflicts         = "accepted_conflicts"
	rejectedConflicts         = "rejected_conflicts"
)

var SlotMetrics = collector.NewCollection(slotNamespace,
	collector.WithMetric(collector.NewMetric(totalBlocks,
		collector.WithType(collector.Counter),
		collector.WithLabels(slotLabelName),
		collector.WithPruningDelay(10*time.Minute),
		collector.WithHelp("Number of blocks seen by the node in a slot."),
		collector.WithInitFunc(func() {
			deps.Protocol.Events.Engine.BlockDAG.BlockAttached.Hook(func(block *blocks.Block) {
				eventSlot := int(block.ID().Slot())
				deps.Collector.Increment(slotNamespace, totalBlocks, strconv.Itoa(eventSlot))
			}, event.WithWorkerPool(Component.WorkerPool))
		}),
	)),

	collector.WithMetric(collector.NewMetric(acceptedBlocksInSlot,
		collector.WithType(collector.Counter),
		collector.WithHelp("Number of accepted blocks in a slot."),
		collector.WithLabels(slotLabelName),
		collector.WithPruningDelay(10*time.Minute),
		collector.WithInitFunc(func() {
			deps.Protocol.Events.Engine.BlockGadget.BlockAccepted.Hook(func(block *blocks.Block) {
				eventSlot := int(block.ID().Slot())
				deps.Collector.Increment(slotNamespace, acceptedBlocksInSlot, strconv.Itoa(eventSlot))
			}, event.WithWorkerPool(Component.WorkerPool))
		}),
	)),
	collector.WithMetric(collector.NewMetric(invalidBlocks,
		collector.WithType(collector.Counter),
		collector.WithLabels(slotLabelName),
		collector.WithPruningDelay(10*time.Minute),
		collector.WithHelp("Number of invalid blocks in a slot."),
		collector.WithInitFunc(func() {
			deps.Protocol.Events.Engine.BlockDAG.BlockInvalid.Hook(func(block *blocks.Block, err error) {
				eventSlot := int(block.ID().Slot())
				deps.Collector.Increment(slotNamespace, invalidBlocks, strconv.Itoa(eventSlot))
			}, event.WithWorkerPool(Component.WorkerPool))
		}),
	)),
	collector.WithMetric(collector.NewMetric(acceptedAttachments,
		collector.WithType(collector.Counter),
		collector.WithLabels(slotLabelName),
		collector.WithPruningDelay(10*time.Minute),
		collector.WithHelp("Number of accepted attachments by the node per slot."),
		collector.WithInitFunc(func() {
			deps.Protocol.Engines.Main.Get().Ledger.OnTransactionAttached(func(transactionMetadata mempool.TransactionMetadata) {
				transactionMetadata.OnAccepted(func() {
					for _, attachmentBlockID := range transactionMetadata.ValidAttachments() {
						if block, exists := deps.Protocol.Engines.Main.Get().BlockCache.Block(attachmentBlockID); exists && block.IsAccepted() {
							deps.Collector.Increment(slotNamespace, acceptedAttachments, strconv.Itoa(int(attachmentBlockID.Slot())))
						}
					}
				})
			})
		}),
	)),
	collector.WithMetric(collector.NewMetric(createdConflicts,
		collector.WithType(collector.Counter),
		collector.WithLabels(slotLabelName),
		collector.WithPruningDelay(10*time.Minute),
		collector.WithHelp("Number of conflicts created per slot."),
		collector.WithInitFunc(func() {
			// Attach to BlockAttached to initialize metric for the slot for improved readability in the Dashboard.
			// Update for a counter doesn't override the value with 0, but rather adds 0 to the value.
			deps.Protocol.Events.Engine.BlockDAG.BlockAttached.Hook(func(block *blocks.Block) {
				eventSlot := int(block.ID().Slot())
				deps.Collector.Update(slotNamespace, createdConflicts, 0, strconv.Itoa(eventSlot))
			}, event.WithWorkerPool(Component.WorkerPool))

			deps.Protocol.Events.Engine.ConflictDAG.ConflictCreated.Hook(func(conflictID iotago.TransactionID) {
				if txMetadata, exists := deps.Protocol.Engines.Main.Get().Ledger.TransactionMetadata(conflictID); exists {
					for _, attachment := range txMetadata.ValidAttachments() {
						deps.Collector.Increment(slotNamespace, createdConflicts, strconv.Itoa(int(attachment.Slot())))
					}
				}
			}, event.WithWorkerPool(Component.WorkerPool))
		}),
	)),
	collector.WithMetric(collector.NewMetric(acceptedConflicts,
		collector.WithType(collector.Counter),
		collector.WithLabels(slotLabelName),
		collector.WithPruningDelay(10*time.Minute),
		collector.WithHelp("Number of conflicts accepted per slot."),
		collector.WithInitFunc(func() {
			// Attach to BlockAttached to initialize metric for the slot for improved readability in the Dashboard.
			// Update for a counter doesn't override the value with 0, but rather adds 0 to the value.
			deps.Protocol.Events.Engine.BlockDAG.BlockAttached.Hook(func(block *blocks.Block) {
				eventSlot := int(block.ID().Slot())
				deps.Collector.Update(slotNamespace, acceptedConflicts, 0, strconv.Itoa(eventSlot))
			}, event.WithWorkerPool(Component.WorkerPool))

			deps.Protocol.Events.Engine.ConflictDAG.ConflictAccepted.Hook(func(conflictID iotago.TransactionID) {
				if txMetadata, exists := deps.Protocol.Engines.Main.Get().Ledger.TransactionMetadata(conflictID); exists {
					for _, attachmentBlockID := range txMetadata.ValidAttachments() {
						if attachment, exists := deps.Protocol.Engines.Main.Get().BlockCache.Block(attachmentBlockID); exists && attachment.IsAccepted() {
							deps.Collector.Increment(slotNamespace, acceptedConflicts, strconv.Itoa(int(attachment.ID().Slot())))
						}
					}
				}
			}, event.WithWorkerPool(Component.WorkerPool))
		}),
	)),
	collector.WithMetric(collector.NewMetric(rejectedConflicts,
		collector.WithType(collector.Counter),
		collector.WithLabels(slotLabelName),
		collector.WithPruningDelay(10*time.Minute),
		collector.WithHelp("Number of conflicts rejected per slot."),
		collector.WithInitFunc(func() {
			// Attach to BlockAttached to initialize metric for the slot for improved readability in the Dashboard.
			// Update for a counter doesn't override the value with 0, but rather adds 0 to the value.
			deps.Protocol.Events.Engine.BlockDAG.BlockAttached.Hook(func(block *blocks.Block) {
				eventSlot := int(block.ID().Slot())
				deps.Collector.Update(slotNamespace, rejectedConflicts, 0, strconv.Itoa(eventSlot))
			}, event.WithWorkerPool(Component.WorkerPool))

			deps.Protocol.Events.Engine.ConflictDAG.ConflictRejected.Hook(func(conflictID iotago.TransactionID) {
				if txMetadata, exists := deps.Protocol.Engines.Main.Get().Ledger.TransactionMetadata(conflictID); exists {
					for _, attachmentBlockID := range txMetadata.ValidAttachments() {
						if attachment, exists := deps.Protocol.Engines.Main.Get().BlockCache.Block(attachmentBlockID); exists && attachment.IsAccepted() {
							deps.Collector.Increment(slotNamespace, rejectedConflicts, strconv.Itoa(int(attachment.ID().Slot())))
						}
					}
				}
			}, event.WithWorkerPool(Component.WorkerPool))
		}),
	)),
)
