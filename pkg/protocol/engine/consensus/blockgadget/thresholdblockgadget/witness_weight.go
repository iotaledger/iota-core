package thresholdblockgadget

import (
	"github.com/pkg/errors"

	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/votes"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (g *Gadget) trackWitnessWeight(votingBlock *blocks.Block) {
	witness := votingBlock.Block().IssuerID

	// Only track witness weight for issuers that are part of the committee.
	if !g.sybilProtection.Committee().Has(witness) {
		return
	}

	// Add the witness to the voting block itself as each block carries a vote for itself.
	if votingBlock.AddWitness(witness) {
		g.tryPreAcceptAndPreConfirm(votingBlock)
	}

	evaluateFunc := func(block *blocks.Block) bool {
		// Skip propagation if the block is already preAccepted.
		if block.IsPreAccepted() {
			return false
		}

		// Skip further propagation if the witness is not new.
		if !block.AddWitness(witness) {
			return false
		}

		g.tryPreAcceptAndPreConfirm(block)

		return true
	}

	g.propagate(votingBlock.Parents(), evaluateFunc)
}

func (g *Gadget) tryPreAcceptAndPreConfirm(block *blocks.Block) {
	committee := g.sybilProtection.Committee()
	committeeTotalWeight := committee.TotalWeight()
	blockWeight := committee.SelectAccounts(block.Witnesses()...).TotalWeight()

	onlineCommittee := g.sybilProtection.OnlineCommittee()
	onlineCommitteeTotalWeight := onlineCommittee.TotalWeight()
	blockWeightOnline := onlineCommittee.SelectAccounts(block.Witnesses()...).TotalWeight()

	if votes.IsThresholdReached(blockWeight, committeeTotalWeight, g.optsConfirmationThreshold) {
		g.propagatePreAcceptanceAndPreConfirmation(block, true)
	} else if votes.IsThresholdReached(blockWeightOnline, onlineCommitteeTotalWeight, g.optsAcceptanceThreshold) {
		g.propagatePreAcceptanceAndPreConfirmation(block, false)
	}
}

func (g *Gadget) propagatePreAcceptanceAndPreConfirmation(initialBlock *blocks.Block, preConfirmed bool) {
	evaluateFunc := func(block *blocks.Block) bool {
		shouldWalkPastCone := false

		if !block.IsPreAccepted() {
			g.preAcceptanceOrder.Queue(block)
			shouldWalkPastCone = true
		}

		if preConfirmed && !block.IsPreConfirmed() {
			g.markAsPreConfirmed(block)
			shouldWalkPastCone = true
		}

		return shouldWalkPastCone
	}

	g.propagate([]iotago.BlockID{initialBlock.ID()}, evaluateFunc)
}

func (g *Gadget) markAsPreAccepted(block *blocks.Block) (err error) {
	if block.SetPreAccepted() {
		g.events.BlockPreAccepted.Trigger(block)

		g.trackAcceptanceRatifierWeight(block)
	}

	return nil
}

func (g *Gadget) markAsPreConfirmed(block *blocks.Block) {
	if block.SetPreConfirmed() {
		g.events.BlockPreConfirmed.Trigger(block)

		g.trackConfirmationRatifierWeight(block)
	}
}

func (g *Gadget) preAcceptanceFailed(block *blocks.Block, err error) {
	panic(errors.Wrapf(err, "could not mark block %s as accepted", block.ID()))
}
