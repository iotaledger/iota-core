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

func (g *Gadget) tryPreAcceptAndPreConfirm(block *blocks.Block) {
	committee := g.sybilProtection.Committee()
	committeeTotalWeight := committee.TotalWeight()
	blockWeight := committee.SelectAccounts(block.Witnesses()...).TotalWeight()

	onlineCommittee := g.sybilProtection.OnlineCommittee()
	onlineCommitteeTotalWeight := onlineCommittee.TotalWeight()
	blockWeightOnline := onlineCommittee.SelectAccounts(block.Witnesses()...).TotalWeight()

	if votes.IsThresholdReached(blockWeight, committeeTotalWeight, g.optsConfirmationThreshold) {
		g.propagatePreAcceptanceAndPreConfirmation(block, true)
	} else if votes.IsThresholdReached(blockWeightOnline, onlineCommitteeTotalWeight, g.optsAcceptanceThreshold) {
		g.propagatePreAcceptanceAndPreConfirmation(block, false)
	}
}

func (g *Gadget) propagatePreAcceptanceAndPreConfirmation(initialBlock *blocks.Block, preConfirmed bool) {
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

		shouldWalkPastCone := false
		if !walkerBlock.IsPreAccepted() {
			g.preAcceptanceOrder.Queue(walkerBlock)
			shouldWalkPastCone = true
		}

		if preConfirmed && !walkerBlock.IsPreConfirmed() {
			g.markAsPreConfirmed(walkerBlock)
			shouldWalkPastCone = true
		}

		if !shouldWalkPastCone {
			continue
		}

		walkerBlock.ForEachParent(func(parent model.Parent) {
			switch parent.Type {
			case model.StrongParentType:
				pastConeWalker.Push(parent.ID)
			case model.ShallowLikeParentType, model.WeakParentType:
				if weakParent, exists := g.blockCache.Block(parent.ID); !exists {
					g.preAcceptanceOrder.Queue(weakParent)
					if preConfirmed {
						g.markAsPreConfirmed(weakParent)
					}
				}
			}
		})
	}
}

func (g *Gadget) markAsPreAccepted(block *blocks.Block) (err error) {
	if block.SetPreAccepted() {
		g.events.BlockPreAccepted.Trigger(block)

		g.trackAcceptanceRatifierWeight(block)
	}

	return nil
}

func (g *Gadget) markAsPreConfirmed(block *blocks.Block) {
	if block.SetPreConfirmed() {
		g.events.BlockPreConfirmed.Trigger(block)

		g.trackConfirmationRatifierWeight(block)
	}
}

func (g *Gadget) preAcceptanceFailed(block *blocks.Block, err error) {
	panic(errors.Wrapf(err, "could not mark block %s as accepted", block.ID()))
}
