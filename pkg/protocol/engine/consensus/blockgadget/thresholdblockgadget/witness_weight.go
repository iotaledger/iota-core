package thresholdblockgadget

import (
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/votes"
)

func (g *Gadget) trackWitnessWeight(votingBlock *blocks.Block) {
	witness := votingBlock.Block().IssuerID

	// Only track witness weight for issuers that are part of the committee.
	if !g.sybilProtection.Committee().Has(witness) {
		return
	}

	var preAcceptanceStack []*blocks.Block
	var preConfirmationStack []*blocks.Block

	process := func(block *blocks.Block) {
		shouldPreAccept, shouldPreConfirm := g.shouldPreAcceptAndPreConfirm(block)

		if !block.IsPreAccepted() && shouldPreAccept {
			preAcceptanceStack = append([]*blocks.Block{block}, preAcceptanceStack...)
		}

		if !block.IsPreConfirmed() && shouldPreConfirm {
			preConfirmationStack = append([]*blocks.Block{block}, preConfirmationStack...)
		}
	}

	// Add the witness to the voting block itself as each block carries a vote for itself.
	if votingBlock.AddWitness(witness) {
		process(votingBlock)
	}

	evaluateFunc := func(block *blocks.Block) bool {
		// Skip further propagation if the witness is not new.
		if !block.AddWitness(witness) {
			return false
		}

		process(block)

		return true
	}

	g.propagate(votingBlock.Parents(), evaluateFunc)

	var acceptanceRatifierWeights []*blocks.Block
	for _, block := range preAcceptanceStack {
		if block.SetPreAccepted() {
			g.events.BlockPreAccepted.Trigger(block)
			acceptanceRatifierWeights = append(acceptanceRatifierWeights, block)
		}
	}

	var confirmationRatifierWeights []*blocks.Block
	for _, block := range preConfirmationStack {
		if block.SetPreConfirmed() {
			g.events.BlockPreConfirmed.Trigger(block)
			confirmationRatifierWeights = append(confirmationRatifierWeights, block)
		}
	}

	for _, block := range acceptanceRatifierWeights {
		g.trackAcceptanceRatifierWeight(block)
	}

	for _, block := range confirmationRatifierWeights {
		g.trackConfirmationRatifierWeight(block)
	}
}

func (g *Gadget) shouldPreAcceptAndPreConfirm(block *blocks.Block) (preAccept bool, preConfirm bool) {
	committee := g.sybilProtection.Committee()
	committeeTotalWeight := committee.TotalWeight()
	blockWeight := committee.SelectAccounts(block.Witnesses()...).TotalWeight()

	onlineCommittee := g.sybilProtection.OnlineCommittee()
	onlineCommitteeTotalWeight := onlineCommittee.TotalWeight()
	blockWeightOnline := onlineCommittee.SelectAccounts(block.Witnesses()...).TotalWeight()

	if votes.IsThresholdReached(blockWeight, committeeTotalWeight, g.optsConfirmationThreshold) {
		return true, true
	} else if votes.IsThresholdReached(blockWeightOnline, onlineCommitteeTotalWeight, g.optsAcceptanceThreshold) {
		return true, false
	}

	return false, false
}
