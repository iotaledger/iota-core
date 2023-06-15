package thresholdblockgadget

import (
	"fmt"

	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/votes"
)

func (g *Gadget) trackAcceptanceRatifierWeight(votingBlock *blocks.Block) {
	ratifier := votingBlock.Block().IssuerID

	// Only track ratifier weight for issuers that are part of the committee.
	if !g.sybilProtection.Committee().Has(ratifier) {
		return
	}

	fmt.Println("\t======== Acceptance =======", votingBlock.ID(), ratifier)

	var stack []*blocks.Block

	evaluateFunc := func(block *blocks.Block) bool {
		fmt.Println("\t\t", block.ID(), "isAccepted", block.IsAccepted(), block.AcceptanceRatifiers(), "shouldAccept", g.shouldAccept(block))
		// Skip propagation if the block is already accepted.
		if block.IsAccepted() {
			return false
		}

		// Propagate further if the ratifier is new.
		propagateFurther := block.AddAcceptanceRatifier(ratifier)

		if g.shouldAccept(block) {
			// We start walking from the future cone into the past in a breadth-first manner. Therefore, we prepend (push onto the stack) here
			// so that we can accept in order after finishing the walk.
			stack = append([]*blocks.Block{block}, stack...)

			// Even if the ratifier is not new, we should accept this block just now (potentially due to OnlineCommittee changes).
			// That means, we should check its parents to ensure monotonicity:
			//  1. If they are not yet accepted, we will add them to the stack and accept them.
			//  2. If they are accepted, we will stop the walk.
			propagateFurther = true
		}

		return propagateFurther
	}

	g.propagate(votingBlock.Parents(), evaluateFunc)

	for _, block := range stack {
		if block.SetAccepted() {
			fmt.Println("\t\t", block.ID(), "accepted")
			g.events.BlockAccepted.Trigger(block)
		}
	}
}

func (g *Gadget) shouldAccept(block *blocks.Block) bool {
	blockWeight := g.sybilProtection.OnlineCommittee().SelectAccounts(block.AcceptanceRatifiers()...).TotalWeight()
	onlineCommitteeTotalWeight := g.sybilProtection.OnlineCommittee().TotalWeight()

	return votes.IsThresholdReached(blockWeight, onlineCommitteeTotalWeight, g.optsAcceptanceThreshold)
}
