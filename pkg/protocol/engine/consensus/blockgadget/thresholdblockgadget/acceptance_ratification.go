package thresholdblockgadget

import (
	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/votes"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (g *Gadget) trackAcceptanceRatifierWeight(votingBlock *blocks.Block) {
	// Only track ratifier weight for issuers that are part of the committee.
	seat, isValid := g.isCommitteeValidationBlock(votingBlock)
	if !isValid {
		return
	}

	var toAccept []*blocks.Block
	toAcceptByID := ds.NewSet[iotago.BlockID]()

	evaluateFunc := func(block *blocks.Block) bool {
		// Skip propagation if the block is already accepted.
		if block.IsAccepted() {
			return false
		}

		// Propagate further if the ratifier is new.
		propagateFurther := block.AddAcceptanceRatifier(seat)

		// Once a block is accepted, all its parents are implicitly accepted as well. There's no need to check shouldAccept again.
		if anyChildInSet(block, toAcceptByID) || g.shouldAccept(block) {
			// We start walking from the future cone into the past in a breadth-first manner. Therefore, we prepend (push onto the toAccept) here
			// so that we can accept in order after finishing the walk.
			toAccept = append([]*blocks.Block{block}, toAccept...)
			toAcceptByID.Add(block.ID())

			// A child of this block has been accepted or this block has just been accepted.
			// That means, we should check its parents to ensure monotonicity:
			//  1. If they are not yet accepted, we will add them to the toAccept and accept them.
			//  2. If they are accepted, we will simply stop the walk.
			propagateFurther = true
		}

		return propagateFurther
	}

	g.propagate(votingBlock.Parents(), evaluateFunc)

	for _, block := range toAccept {
		if block.SetAccepted() {
			g.events.BlockAccepted.Trigger(block)
		}
	}
}

func (g *Gadget) shouldAccept(block *blocks.Block) bool {
	blockSeats := block.AcceptanceRatifiers().Size()
	onlineCommitteeTotalSeats := g.seatManager.OnlineCommittee().Size()

	return votes.IsThresholdReached(blockSeats, onlineCommitteeTotalSeats, g.optsAcceptanceThreshold)
}

func (g *Gadget) SetAccepted(block *blocks.Block) bool {
	if block.SetAccepted() {
		g.events.BlockAccepted.Trigger(block)
		return true
	}

	return false
}
