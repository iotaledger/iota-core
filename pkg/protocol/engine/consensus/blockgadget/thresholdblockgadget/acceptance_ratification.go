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

		// Skip propagation if the block is already ratified accepted.
		if block.IsRatifiedAccepted() {
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
						g.tryRatifyAccept(weakParent)
					}
				}
			}
		})

		g.tryRatifyAccept(block)
	}
}

func (g *Gadget) tryRatifyAccept(block *blocks.Block) {
	blockWeight := g.sybilProtection.OnlineCommittee().SelectAccounts(block.AcceptanceRatifiers()...).TotalWeight()
	onlineCommitteeTotalWeight := g.sybilProtection.OnlineCommittee().TotalWeight()

	if votes.IsThresholdReached(blockWeight, onlineCommitteeTotalWeight, g.optsAcceptanceThreshold) {
		g.propagateRatifiedAcceptance(block)
	}
}

func (g *Gadget) propagateRatifiedAcceptance(initialBlock *blocks.Block) {
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

		if walkerBlock.IsRatifiedAccepted() {
			continue
		}

		g.ratifiedAcceptanceOrder.Queue(walkerBlock)

		walkerBlock.ForEachParent(func(parent model.Parent) {
			switch parent.Type {
			case model.StrongParentType:
				pastConeWalker.Push(parent.ID)
			case model.ShallowLikeParentType, model.WeakParentType:
				if weakParent, exists := g.blockCache.Block(parent.ID); !exists {
					g.ratifiedAcceptanceOrder.Queue(weakParent)
				}
			}
		})
	}
}

func (g *Gadget) markAsRatifiedAccepted(block *blocks.Block) (err error) {
	if block.SetRatifiedAccepted() {
		g.events.BlockRatifiedAccepted.Trigger(block)
	}

	return nil
}

func (g *Gadget) ratifiedAcceptanceFailed(block *blocks.Block, err error) {
	panic(errors.Wrapf(err, "could not mark block %s as ratified accepted", block.ID()))
}
