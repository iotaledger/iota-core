package thresholdblockgadget

import (
	"github.com/pkg/errors"

	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/votes"
)

func (g *Gadget) trackAcceptanceRatifierWeight(votingBlock *blocks.Block) {
	ratifier := votingBlock.Block().IssuerID

	// Only track ratifier weight for issuers that are part of the committee.
	if !g.sybilProtection.Committee().Has(ratifier) {
		return
	}

	evaluateFunc := func(block *blocks.Block) bool {
		// Skip propagation if the block is already accepted.
		if block.IsAccepted() {
			return false
		}

		// Skip further propagation if the witness is not new.
		if !block.AddAcceptanceRatifier(ratifier) {
			return false
		}

		g.tryAccept(block)

		return true
	}

	g.propagate(votingBlock.Parents(), evaluateFunc)
}

func (g *Gadget) tryAccept(block *blocks.Block) {
	blockWeight := g.sybilProtection.OnlineCommittee().SelectAccounts(block.AcceptanceRatifiers()...).TotalWeight()
	onlineCommitteeTotalWeight := g.sybilProtection.OnlineCommittee().TotalWeight()

	if votes.IsThresholdReached(blockWeight, onlineCommitteeTotalWeight, g.optsAcceptanceThreshold) {
		g.acceptanceOrder.Queue(block)
	}
}

func (g *Gadget) markAsAccepted(block *blocks.Block) (err error) {
	if block.SetAccepted() {
		g.events.BlockAccepted.Trigger(block)
	}

	return nil
}

func (g *Gadget) acceptanceFailed(block *blocks.Block, err error) {
	panic(errors.Wrapf(err, "could not mark block %s as accepted", block.ID()))
}
