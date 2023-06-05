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

func (g *Gadget) trackConfirmationRatifierWeight(votingBlock *blocks.Block) {
	ratifier := votingBlock.Block().IssuerID

	// Only track ratifier weight for issuers that are part of the committee.
	if !g.sybilProtection.Committee().Has(ratifier) {
		return
	}

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

		if block.IsRatifiedConfirmed() {
			continue
		}

		fmt.Println("trackConfirmationRatifierWeight", blockID, ratifier)
		// Skip further propagation if the ratifier is not new.
		if !block.AddConfirmationRatifier(ratifier) {
			continue
		}

		block.ForEachParent(func(parent model.Parent) {
			switch parent.Type {
			case model.StrongParentType:
				walk.Push(parent.ID)
			case model.ShallowLikeParentType, model.WeakParentType:
				if weakParent, exists := g.blockCache.Block(parent.ID); exists {
					if weakParent.AddConfirmationRatifier(ratifier) {
						g.tryRatifyConfirm(weakParent)
					}
				}
			}
		})

		g.tryRatifyConfirm(block)
	}
}

func (g *Gadget) tryRatifyConfirm(block *blocks.Block) {
	blockWeight := g.sybilProtection.Committee().SelectAccounts(block.ConfirmationRatifiers()...).TotalWeight()
	totalCommitteeWeight := g.sybilProtection.Committee().TotalWeight()

	if votes.IsThresholdReached(blockWeight, totalCommitteeWeight, g.optsConfirmationThreshold) {
		g.propagateRatifiedConfirmation(block)
	}
}

func (g *Gadget) propagateRatifiedConfirmation(initialBlock *blocks.Block) {
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

		if walkerBlock.IsRatifiedConfirmed() {
			continue
		}

		g.ratifiedConfirmationOrder.Queue(walkerBlock)

		walkerBlock.ForEachParent(func(parent model.Parent) {
			switch parent.Type {
			case model.StrongParentType:
				pastConeWalker.Push(parent.ID)
			case model.ShallowLikeParentType, model.WeakParentType:
				if weakParent, exists := g.blockCache.Block(parent.ID); !exists {
					g.ratifiedConfirmationOrder.Queue(weakParent)
				}
			}
		})
	}
}

func (g *Gadget) markAsRatifiedConfirmed(block *blocks.Block) (err error) {
	if block.SetRatifiedConfirmed() {
		g.events.BlockRatifiedConfirmed.Trigger(block)
	}

	return nil
}

func (g *Gadget) ratifiedConfirmationFailed(block *blocks.Block, err error) {
	panic(errors.Wrapf(err, "could not mark block %s as ratified confirmed", block.ID()))
}
