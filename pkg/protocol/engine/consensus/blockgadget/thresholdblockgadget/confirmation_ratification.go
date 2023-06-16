package thresholdblockgadget

import (
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/votes"
)

func (g *Gadget) trackConfirmationRatifierWeight(votingBlock *blocks.Block) {
	ratifier := votingBlock.Block().IssuerID
	ratifierBlockIndex := votingBlock.ID().Index()

	// Only track ratifier weight for issuers that are part of the committee.
	if !g.sybilProtection.Committee().Has(ratifier) {
		return
	}

	var stack []*blocks.Block

	evaluateFunc := func(block *blocks.Block) bool {
		// Do not propagate further than g.optsConfirmationRatificationThreshold slots.
		// This means that confirmations need to be achieved within g.optsConfirmationRatificationThreshold slots.
		if ratifierBlockIndex >= g.optsConfirmationRatificationThreshold &&
			block.ID().Index() <= ratifierBlockIndex-g.optsConfirmationRatificationThreshold {
			return false
		}

		// Skip propagation if the block is already accepted.
		if block.IsConfirmed() {
			return false
		}

		// Skip further propagation if the witness is not new.
		if !block.AddConfirmationRatifier(ratifier) {
			return false
		}

		if g.shouldConfirm(block) {
			stack = append([]*blocks.Block{block}, stack...)
		}

		return true
	}

	g.propagate(votingBlock.Parents(), evaluateFunc)

	for _, block := range stack {
		if block.SetConfirmed() {
			g.events.BlockConfirmed.Trigger(block)
		}
	}
}

func (g *Gadget) shouldConfirm(block *blocks.Block) bool {
	blockWeight := g.sybilProtection.Committee().SelectAccounts(block.ConfirmationRatifiers()...).TotalWeight()
	totalCommitteeWeight := g.sybilProtection.Committee().TotalWeight()

	return votes.IsThresholdReached(blockWeight, totalCommitteeWeight, g.optsConfirmationThreshold)
}
