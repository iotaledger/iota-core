package protocol

import (
	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Status struct {
	NodeSynced             bool
	LastAcceptedBlockSlot  iotago.SlotIndex
	LastConfirmedBlockSlot iotago.SlotIndex
	LatestFinalizedSlot    iotago.SlotIndex
	LatestPrunedSlot       iotago.SlotIndex
	LatestCommitment       *model.Commitment
}

func newStatusVariable(p *Protocol) reactive.Variable[Status] {
	s := reactive.NewVariable[Status]()

	var unsubscribeFromPreviousEngine func()

	p.MainEngineR().OnUpdate(func(_, engine *engine.Engine) {
		if unsubscribeFromPreviousEngine != nil {
			unsubscribeFromPreviousEngine()
		}

		s.Set(Status{
			NodeSynced:             engine.IsSynced(),
			LastAcceptedBlockSlot:  engine.BlockGadget.LastAcceptedBlockIndex(),
			LastConfirmedBlockSlot: engine.BlockGadget.LastConfirmedBlockIndex(),
			LatestFinalizedSlot:    engine.Storage.Settings().LatestFinalizedSlot(),
			LatestPrunedSlot:       lo.Return1(engine.Storage.Prunable.LastPrunedSlot()),
			LatestCommitment:       engine.Storage.Settings().LatestCommitment(),
		})

		unsubscribeFromPreviousEngine = lo.Batch(
			// TODO: Update NodeSynced when the node is bootstrapped

			engine.Events.BlockGadget.BlockAccepted.Hook(func(acceptedBlock *blocks.Block) {
				s.Compute(func(status Status) Status {
					status.LastAcceptedBlockSlot = acceptedBlock.ID().Index()
					return status
				})
			}).Unhook,

			engine.Events.BlockGadget.BlockConfirmed.Hook(func(confirmedBlock *blocks.Block) {
				s.Compute(func(status Status) Status {
					status.LastConfirmedBlockSlot = confirmedBlock.ID().Index()
					return status
				})
			}).Unhook,

			engine.Events.SlotGadget.SlotFinalized.Hook(func(latestFinalizedSlot iotago.SlotIndex) {
				s.Compute(func(status Status) Status {
					status.LatestFinalizedSlot = latestFinalizedSlot
					return status
				})
			}).Unhook,

			engine.Events.StoragePruned.Hook(func(latestPrunedSlot iotago.SlotIndex) {
				s.Compute(func(status Status) Status {
					status.LatestPrunedSlot = latestPrunedSlot
					return status
				})
			}).Unhook,

			engine.Events.Notarization.LatestCommitmentUpdated.Hook(func(latestCommitment *model.Commitment) {
				s.Compute(func(status Status) Status {
					status.LatestCommitment = latestCommitment
					return status
				})
			}).Unhook,
		)
	})

	return s
}
