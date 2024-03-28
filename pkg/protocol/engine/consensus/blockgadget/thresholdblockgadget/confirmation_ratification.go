package thresholdblockgadget

import (
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/votes"
)

func (g *Gadget) trackConfirmationRatifierWeight(votingBlock *blocks.Block) {
	// Only track ratifier weight for issuers that are part of the committee.
	seat, isValid := g.isCommitteeValidationBlock(votingBlock)
	if !isValid {
		return
	}

	ratifierBlockIndex := votingBlock.ID().Slot()
	ratifierBlockEpoch := votingBlock.ProtocolBlock().API.TimeProvider().EpochFromSlot(ratifierBlockIndex)

	var toConfirm []*blocks.Block

	evaluateFunc := func(block *blocks.Block) bool {
		// Do not propagate further than g.optsConfirmationRatificationThreshold slots.
		// This means that confirmations need to be achieved within g.optsConfirmationRatificationThreshold slots.
		if ratifierBlockIndex >= g.optsConfirmationRatificationThreshold &&
			block.ID().Slot() <= ratifierBlockIndex-g.optsConfirmationRatificationThreshold {
			return false
		}

		// Skip propagation if the block is not in the same epoch as the ratifier. This might delay the confirmation
		// of blocks at the end of the epoch but make sure that confirmation is safe in case where the minority of voters
		// got different seats for the next epoch.
		blockEpoch := block.ProtocolBlock().API.TimeProvider().EpochFromSlot(block.ID().Slot())
		if ratifierBlockEpoch != blockEpoch {
			return false
		}

		// Skip propagation if the block is already confirmed.
		if block.IsConfirmed() {
			return false
		}

		// Skip further propagation if the ratifier is not new.
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
	blockSeats := block.ConfirmationRatifiers().Size()
	totalCommitteeSeats := g.seatManager.SeatCountInSlot(block.ID().Slot())

	return votes.IsThresholdReached(blockSeats, totalCommitteeSeats, g.optsConfirmationThreshold)
}
