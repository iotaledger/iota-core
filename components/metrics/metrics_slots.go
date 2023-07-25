package metrics

import (
	"strconv"

	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/iota-core/components/metrics/collector"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/notarization"
	iotago "github.com/iotaledger/iota.go/v4"
)

const (
	slotNamespace             = "slots"
	labelName                 = "slot"
	metricEvictionOffset      = 6
	totalBlocks               = "total_blocks"
	acceptedBlocksInSlot      = "accepted_blocks"
	orphanedBlocks            = "orphaned_blocks"
	invalidBlocks             = "invalid_blocks"
	subjectivelyInvalidBlocks = "subjectively_invalid_blocks"
	totalAttachments          = "total_attachments"
	rejectedAttachments       = "rejected_attachments"
	acceptedAttachments       = "accepted_attachments"
	orphanedAttachments       = "orphaned_attachments"
	createdConflicts          = "created_conflicts"
	acceptedConflicts         = "accepted_conflicts"
	rejectedConflicts         = "rejected_conflicts"
)

var SlotMetrics = collector.NewCollection(slotNamespace,
	collector.WithMetric(collector.NewMetric(totalBlocks,
		collector.WithType(collector.CounterVec),
		collector.WithLabels(labelName),
		collector.WithHelp("Number of blocks seen by the node in a slot."),
		collector.WithInitFunc(func() {
			deps.Protocol.Events.Engine.BlockDAG.BlockAttached.Hook(func(block *blocks.Block) {
				eventSlot := int(block.ID().Index())
				deps.Collector.Increment(slotNamespace, totalBlocks, strconv.Itoa(eventSlot))

				// need to initialize slot metrics with 0 to have consistent data for each slot
				for _, metricName := range []string{acceptedBlocksInSlot, orphanedBlocks, invalidBlocks, subjectivelyInvalidBlocks, totalAttachments, orphanedAttachments, rejectedAttachments, acceptedAttachments, createdConflicts, acceptedConflicts, rejectedConflicts} {
					deps.Collector.Update(slotNamespace, metricName, map[string]float64{
						strconv.Itoa(eventSlot): 0,
					})
				}
			}, event.WithWorkerPool(Component.WorkerPool))

			// initialize it once and remove committed slot from all metrics (as they will not change afterwards)
			// in a single attachment instead of multiple ones
			deps.Protocol.Events.Engine.Notarization.SlotCommitted.Hook(func(details *notarization.SlotCommittedDetails) {
				slotToEvict := int(details.Commitment.Index()) - metricEvictionOffset

				// need to remove metrics for old slots, otherwise they would be stored in memory and always exposed to Prometheus, forever
				for _, metricName := range []string{totalBlocks, acceptedBlocksInSlot, orphanedBlocks, invalidBlocks, subjectivelyInvalidBlocks, totalAttachments, orphanedAttachments, rejectedAttachments, acceptedAttachments, createdConflicts, acceptedConflicts, rejectedConflicts} {
					deps.Collector.ResetMetricLabels(slotNamespace, metricName, map[string]string{
						labelName: strconv.Itoa(slotToEvict),
					})
				}
			}, event.WithWorkerPool(Component.WorkerPool))
		}),
	)),

	collector.WithMetric(collector.NewMetric(acceptedBlocksInSlot,
		collector.WithType(collector.CounterVec),
		collector.WithHelp("Number of accepted blocks in a slot."),
		collector.WithLabels(labelName),
		collector.WithInitFunc(func() {
			deps.Protocol.Events.Engine.BlockGadget.BlockAccepted.Hook(func(block *blocks.Block) {
				eventSlot := int(block.ID().Index())
				deps.Collector.Increment(slotNamespace, acceptedBlocksInSlot, strconv.Itoa(eventSlot))
			}, event.WithWorkerPool(Component.WorkerPool))
		}),
	)),
	// collector.WithMetric(collector.NewMetric(orphanedBlocks,
	// 	collector.WithType(collector.CounterVec),
	// 	collector.WithLabels(labelName),
	// 	collector.WithHelp("Number of orphaned blocks in a slot."),
	// 	collector.WithInitFunc(func() {
	// 		deps.Protocol.Events.Engine.BlockDAG.BlockOrphaned.Hook(func(block *blockdag.Block) {
	// 			eventSlot := int(block.ID().Index())
	// 			deps.Collector.Increment(slotNamespace, orphanedBlocks, strconv.Itoa(eventSlot))
	// 		}, event.WithWorkerPool(Component.WorkerPool))
	// 	}),
	// )),
	collector.WithMetric(collector.NewMetric(invalidBlocks,
		collector.WithType(collector.CounterVec),
		collector.WithLabels(labelName),
		collector.WithHelp("Number of invalid blocks in a slot."),
		collector.WithInitFunc(func() {
			deps.Protocol.Events.Engine.BlockDAG.BlockInvalid.Hook(func(block *blocks.Block, err error) {
				eventSlot := int(block.ID().Index())
				deps.Collector.Increment(slotNamespace, invalidBlocks, strconv.Itoa(eventSlot))
			}, event.WithWorkerPool(Component.WorkerPool))
		}),
	)),
	// collector.WithMetric(collector.NewMetric(subjectivelyInvalidBlocks,
	// 	collector.WithType(collector.CounterVec),
	// 	collector.WithLabels(labelName),
	// 	collector.WithHelp("Number of invalid blocks in a slot slot."),
	// 	collector.WithInitFunc(func() {
	// 		deps.Protocol.Events.Engine.Booker.BlockTracked.Hook(func(block *booker.Block) {
	// 			if block.IsSubjectivelyInvalid() {
	// 				eventSlot := int(block.ID().Index())
	// 				deps.Collector.Increment(slotNamespace, subjectivelyInvalidBlocks, strconv.Itoa(eventSlot))
	// 			}
	// 		}, event.WithWorkerPool(Component.WorkerPool))
	// 	}),
	// )),

	// collector.WithMetric(collector.NewMetric(totalAttachments,
	// 	collector.WithType(collector.CounterVec),
	// 	collector.WithLabels(labelName),
	// 	collector.WithHelp("Number of transaction attachments by the node per slot."),
	// 	collector.WithInitFunc(func() {
	// 		deps.Protocol.Events.Engine.Tangle.Booker.AttachmentCreated.Hook(func(block *booker.Block) {
	// 			eventSlot := int(block.ID().Index())
	// 			deps.Collector.Increment(slotNamespace, totalAttachments, strconv.Itoa(eventSlot))
	// 		}, event.WithWorkerPool(Component.WorkerPool))
	// 	}),
	// )),
	// collector.WithMetric(collector.NewMetric(orphanedAttachments,
	// 	collector.WithType(collector.CounterVec),
	// 	collector.WithLabels(labelName),
	// 	collector.WithHelp("Number of orphaned attachments by the node per slot."),
	// 	collector.WithInitFunc(func() {
	// 		deps.Protocol.Events.Engine.Tangle.Booker.AttachmentOrphaned.Hook(func(block *booker.Block) {
	// 			eventSlot := int(block.ID().Index())
	// 			deps.Collector.Increment(slotNamespace, orphanedAttachments, strconv.Itoa(eventSlot))
	// 		}, event.WithWorkerPool(Component.WorkerPool))
	// 	}),
	// )),
	collector.WithMetric(collector.NewMetric(acceptedAttachments,
		collector.WithType(collector.CounterVec),
		collector.WithLabels(labelName),
		collector.WithHelp("Number of rejected attachments by the node per slot."),
		collector.WithInitFunc(func() {
			deps.Protocol.MainEngineInstance().Ledger.OnTransactionAttached(func(transactionMetadata mempool.TransactionMetadata) {
				transactionMetadata.OnAccepted(func() {
					for _, attachmentBlockID := range transactionMetadata.Attachments() {
						if block, exists := deps.Protocol.MainEngineInstance().BlockCache.Block(attachmentBlockID); exists && block.IsAccepted() {
							deps.Collector.Increment(slotNamespace, acceptedAttachments, strconv.Itoa(int(attachmentBlockID.Index())))
						}
					}
				})
			})
		}),
	)),
	collector.WithMetric(collector.NewMetric(createdConflicts,
		collector.WithType(collector.CounterVec),
		collector.WithLabels(labelName),
		collector.WithHelp("Number of conflicts created per slot."),
		collector.WithInitFunc(func() {
			deps.Protocol.Events.Engine.ConflictDAG.ConflictCreated.Hook(func(conflictID iotago.TransactionID) {
				if txMetadata, exists := deps.Protocol.MainEngineInstance().Ledger.TransactionMetadata(conflictID); exists {
					for _, attachment := range txMetadata.Attachments() {
						deps.Collector.Increment(slotNamespace, createdConflicts, strconv.Itoa(int(attachment.Index())))
					}
				}
			}, event.WithWorkerPool(Component.WorkerPool))
		}),
	)),
	collector.WithMetric(collector.NewMetric(acceptedConflicts,
		collector.WithType(collector.CounterVec),
		collector.WithLabels(labelName),
		collector.WithHelp("Number of conflicts accepted per slot."),
		collector.WithInitFunc(func() {
			deps.Protocol.Events.Engine.ConflictDAG.ConflictAccepted.Hook(func(conflictID iotago.TransactionID) {
				if txMetadata, exists := deps.Protocol.MainEngineInstance().Ledger.TransactionMetadata(conflictID); exists {
					for _, attachmentBlockID := range txMetadata.Attachments() {
						if attachment, exists := deps.Protocol.MainEngineInstance().BlockCache.Block(attachmentBlockID); exists && attachment.IsAccepted() {
							deps.Collector.Increment(slotNamespace, acceptedConflicts, strconv.Itoa(int(attachment.ID().Index())))
						}
					}
				}
			}, event.WithWorkerPool(Component.WorkerPool))
		}),
	)),
	// collector.WithMetric(collector.NewMetric(rejectedConflicts,
	// 	collector.WithType(collector.CounterVec),
	// 	collector.WithLabels(labelName),
	// 	collector.WithHelp("Number of conflicts rejected per slot."),
	// 	collector.WithInitFunc(func() {
	// 		deps.Protocol.Events.Engine.ConflictDAG.ConflictAccepted.Hook(func(conflictID iotago.TransactionID) {
	// 			if txMetadata, exists := deps.Protocol.MainEngineInstance().Ledger.TransactionMetadata(conflictID); exists {
	// 				for _, attachmentBlockID := range txMetadata.Attachments() {
	// 					if attachment, exists := deps.Protocol.MainEngineInstance().BlockCache.Block(attachmentBlockID); exists && attachment.IsR() {
	// 						deps.Collector.Increment(slotNamespace, acceptedConflicts, strconv.Itoa(int(attachment.ID().Index())))
	// 					}
	// 				}
	// 			}
	// 		}, event.WithWorkerPool(Component.WorkerPool))
	// 	}),
	// )),
)
