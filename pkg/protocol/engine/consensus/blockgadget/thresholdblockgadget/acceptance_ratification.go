package thresholdblockgadget

import (
	"fmt"

	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/ds/walker"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/votes"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (g *Gadget) trackAcceptanceRatifierWeight(votingBlock *blocks.Block) {
	ratifier := votingBlock.Block().IssuerID

	// Only track ratifier weight for issuers that are part of the committee.
	if !g.sybilProtection.Committee().Has(ratifier) {
		return
	}

	// Walk the block's past cone until we reach an accepted or already supported (by this ratifier) block.
	walk := walker.New[iotago.BlockID]().PushAll(votingBlock.Parents()...)
	for walk.HasNext() {
		blockID := walk.Next()
		block, exists := g.blockCache.Block(blockID)
		if !exists {
			panic(fmt.Sprintf("parent %s does not exist", blockID))
		}

		if block.IsRootBlock() {
			continue
		}

		// Skip propagation if the block is already accepted.
		if block.IsAccepted() {
			continue
		}

		// Skip further propagation if the ratifier is not new.
		if !block.AddAcceptanceRatifier(ratifier) {
			continue
		}

		block.ForEachParent(func(parent model.Parent) {
			switch parent.Type {
			case model.StrongParentType:
				walk.Push(parent.ID)
			case model.ShallowLikeParentType, model.WeakParentType:
				if weakParent, exists := g.blockCache.Block(parent.ID); exists {
					if weakParent.AddAcceptanceRatifier(ratifier) {
						g.tryAccept(weakParent)
					}
				}
			}
		})

		g.tryAccept(block)
	}
}

func (g *Gadget) tryAccept(block *blocks.Block) {
	blockWeight := g.sybilProtection.OnlineCommittee().SelectAccounts(block.AcceptanceRatifiers()...).TotalWeight()
	onlineCommitteeTotalWeight := g.sybilProtection.OnlineCommittee().TotalWeight()

	if votes.IsThresholdReached(blockWeight, onlineCommitteeTotalWeight, g.optsAcceptanceThreshold) {
		g.propagateAcceptance(block)
	}
}

func (g *Gadget) propagateAcceptance(initialBlock *blocks.Block) {
	pastConeWalker := walker.New[iotago.BlockID](false).Push(initialBlock.ID())
	for pastConeWalker.HasNext() {
		blockID := pastConeWalker.Next()
		walkerBlock, exists := g.blockCache.Block(blockID)
		if !exists {
			panic(fmt.Sprintf("parent %s does not exist", blockID))
		}

		if walkerBlock.IsRootBlock() {
			continue
		}

		if walkerBlock.IsAccepted() {
			continue
		}

		g.acceptanceOrder.Queue(walkerBlock)

		walkerBlock.ForEachParent(func(parent model.Parent) {
			switch parent.Type {
			case model.StrongParentType:
				pastConeWalker.Push(parent.ID)
			case model.ShallowLikeParentType, model.WeakParentType:
				if weakParent, exists := g.blockCache.Block(parent.ID); !exists {
					g.acceptanceOrder.Queue(weakParent)
				}
			}
		})
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
