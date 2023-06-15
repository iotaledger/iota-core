package thresholdblockgadget

import (
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/votes"
)

func (g *Gadget) trackAcceptanceRatifierWeight(votingBlock *blocks.Block) {
	ratifier := votingBlock.Block().IssuerID

	// Only track ratifier weight for issuers that are part of the committee.
	if !g.sybilProtection.Committee().Has(ratifier) {
		return
	}

	var stack []*blocks.Block

	evaluateFunc := func(block *blocks.Block) bool {
		// Skip propagation if the block is already accepted.
		if block.IsAccepted() {
			return false
		}

		// Skip further propagation if the witness is not new.
		if !block.AddAcceptanceRatifier(ratifier) {
			return false
		}

		if g.shouldAccept(block) {
			stack = append([]*blocks.Block{block}, stack...)
		}

		return true
	}

	g.propagate(votingBlock.Parents(), evaluateFunc)

	for _, block := range stack {
		if block.SetAccepted() {
			g.events.BlockAccepted.Trigger(block)
		}
	}
}

func (g *Gadget) shouldAccept(block *blocks.Block) bool {
	blockWeight := g.sybilProtection.OnlineCommittee().SelectAccounts(block.AcceptanceRatifiers()...).TotalWeight()
	onlineCommitteeTotalWeight := g.sybilProtection.OnlineCommittee().TotalWeight()

	return votes.IsThresholdReached(blockWeight, onlineCommitteeTotalWeight, g.optsAcceptanceThreshold)
}
