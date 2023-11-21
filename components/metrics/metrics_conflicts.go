package metrics

import (
	"time"

	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/iota-core/components/metrics/collector"
	iotago "github.com/iotaledger/iota.go/v4"
)

const (
	conflictNamespace = "conflict"

	resolutionTime        = "resolution_time_seconds_total"
	allConflictCounts     = "created_total"
	resolvedConflictCount = "resolved_total"
)

var ConflictMetrics = collector.NewCollection(conflictNamespace,
	collector.WithMetric(collector.NewMetric(resolutionTime,
		collector.WithType(collector.Counter),
		collector.WithHelp("Time since transaction issuance to the conflict acceptance"),
		collector.WithInitFunc(func() {
			deps.Protocol.Events.Engine.SpendDAG.SpenderAccepted.Hook(func(spendID iotago.TransactionID) {
				if txMetadata, exists := deps.Protocol.MainEngineInstance().Ledger.MemPool().TransactionMetadata(spendID); exists {
					firstAttachmentID := txMetadata.EarliestIncludedAttachment()
					if block, blockExists := deps.Protocol.MainEngineInstance().BlockFromCache(firstAttachmentID); blockExists {
						timeSinceIssuance := time.Since(block.IssuingTime()).Milliseconds()
						timeIssuanceSeconds := float64(timeSinceIssuance) / 1000
						deps.Collector.Update(conflictNamespace, resolutionTime, timeIssuanceSeconds)
					}
				}
			}, event.WithWorkerPool(Component.WorkerPool))
		}),
	)),
	collector.WithMetric(collector.NewMetric(resolvedConflictCount,
		collector.WithType(collector.Counter),
		collector.WithHelp("Number of resolved (accepted) conflicts"),
		collector.WithInitFunc(func() {
			deps.Protocol.Events.Engine.SpendDAG.SpenderAccepted.Hook(func(spendID iotago.TransactionID) {
				deps.Collector.Increment(conflictNamespace, resolvedConflictCount)
			}, event.WithWorkerPool(Component.WorkerPool))
		}),
	)),
	collector.WithMetric(collector.NewMetric(allConflictCounts,
		collector.WithType(collector.Counter),
		collector.WithHelp("Number of created conflicts"),
		collector.WithInitFunc(func() {
			deps.Protocol.Events.Engine.SpendDAG.SpenderCreated.Hook(func(spendID iotago.TransactionID) {
				deps.Collector.Increment(conflictNamespace, allConflictCounts)
			}, event.WithWorkerPool(Component.WorkerPool))
		}),
	)),
)
