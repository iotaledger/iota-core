package metrics

import (
	"strconv"
	"time"

	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/iota-core/components/metrics/collector"
	"github.com/iotaledger/iota-core/pkg/protocol/chainmanager"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/notarization"
	iotago "github.com/iotaledger/iota.go/v4"
)

const (
	commitmentsNamespace = "commitments"

	latestCommitment    = "latest"
	finalizedCommitment = "finalized"
	forksCount          = "forks_total"
	acceptedBlocks      = "accepted_blocks"
	transactions        = "accepted_transactions"
	validators          = "active_validators"
)

var CommitmentsMetrics = collector.NewCollection(commitmentsNamespace,
	collector.WithMetric(collector.NewMetric(latestCommitment,
		collector.WithType(collector.Gauge),
		collector.WithHelp("Last commitment of the node."),
		collector.WithLabels("commitment"),
		collector.WithPruningDelay(10*time.Minute),
		collector.WithInitFunc(func() {
			deps.Protocol.Events.Engine.Notarization.SlotCommitted.Hook(func(details *notarization.SlotCommittedDetails) {
				deps.Collector.Update(commitmentsNamespace, latestCommitment, float64(details.Commitment.ID().Slot()), details.Commitment.ID().String())
			}, event.WithWorkerPool(Component.WorkerPool))
		}),
	)),
	collector.WithMetric(collector.NewMetric(finalizedCommitment,
		collector.WithType(collector.Gauge),
		collector.WithHelp("Last commitment finalized by the node."),
		collector.WithInitFunc(func() {
			deps.Protocol.Events.Engine.SlotGadget.SlotFinalized.Hook(func(slot iotago.SlotIndex) {
				deps.Collector.Update(commitmentsNamespace, finalizedCommitment, float64(slot))
			}, event.WithWorkerPool(Component.WorkerPool))
		}),
	)),
	collector.WithMetric(collector.NewMetric(forksCount,
		collector.WithType(collector.Counter),
		collector.WithHelp("Number of forks seen by the node."),
		collector.WithInitFunc(func() {
			deps.Protocol.Events.ChainManager.ForkDetected.Hook(func(_ *chainmanager.Fork) {
				deps.Collector.Increment(commitmentsNamespace, forksCount)
			}, event.WithWorkerPool(Component.WorkerPool))
		}),
	)),
	collector.WithMetric(collector.NewMetric(acceptedBlocks,
		collector.WithType(collector.Gauge),
		collector.WithHelp("Number of accepted blocks by the node per slot."),
		collector.WithLabels("slot"),
		collector.WithPruningDelay(10*time.Minute),
		collector.WithResetBeforeCollecting(true),
		collector.WithInitFunc(func() {
			deps.Protocol.Events.Engine.Notarization.SlotCommitted.Hook(func(details *notarization.SlotCommittedDetails) {
				deps.Collector.Update(commitmentsNamespace, acceptedBlocks, float64(details.AcceptedBlocks.Size()), strconv.Itoa(int(details.Commitment.Slot())))
			}, event.WithWorkerPool(Component.WorkerPool))
		}),
	)),
	collector.WithMetric(collector.NewMetric(validators,
		collector.WithType(collector.Gauge),
		collector.WithHelp("Number of active validators per slot."),
		collector.WithLabels("slot"),
		collector.WithPruningDelay(10*time.Minute),
		collector.WithResetBeforeCollecting(true),
		collector.WithInitFunc(func() {
			deps.Protocol.Events.Engine.Notarization.SlotCommitted.Hook(func(details *notarization.SlotCommittedDetails) {
				deps.Collector.Update(commitmentsNamespace, validators, float64(details.ActiveValidatorsCount), strconv.Itoa(int(details.Commitment.Slot())))
			}, event.WithWorkerPool(Component.WorkerPool))
		}),
	)),
)
