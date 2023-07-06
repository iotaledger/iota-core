package thresholdblockgadget

import (
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/votes"
)

func (g *Gadget) trackConfirmationRatifierWeight(votingBlock *blocks.Block) {
	ratifier := votingBlock.ProtocolBlock().IssuerID
	ratifierBlockIndex := votingBlock.ID().Index()

	// Only track ratifier weight for issuers that are part of the committee.
	seat, exists := g.seatManager.Committee(votingBlock.ID().Index()).GetSeat(ratifier)
	if !exists {
		return
	}

	var toConfirm []*blocks.Block

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
		propagateFurther := block.AddConfirmationRatifier(seat)

		if g.shouldConfirm(block) {
			toConfirm = append([]*blocks.Block{block}, toConfirm...)
			propagateFurther = true
		}

		return propagateFurther
	}

	g.propagate(votingBlock.Parents(), evaluateFunc)

	for _, block := range toConfirm {
		if block.SetConfirmed() {
			g.events.BlockConfirmed.Trigger(block)
		}
	}
}

func (g *Gadget) shouldConfirm(block *blocks.Block) bool {
	blockSeats := len(block.ConfirmationRatifiers())
	totalCommitteeSeats := g.seatManager.SeatCount()

	return votes.IsThresholdReached(blockSeats, totalCommitteeSeats, g.optsConfirmationThreshold)
}
