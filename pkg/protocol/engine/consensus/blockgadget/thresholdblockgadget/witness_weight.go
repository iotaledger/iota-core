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

func (g *Gadget) tryAcceptAndConfirm(block *blocks.Block) {
	committee := g.sybilProtection.Committee()
	committeeTotalWeight := committee.TotalWeight()
	blockWeight := committee.SelectAccounts(block.Witnesses()...).TotalWeight()

	onlineCommittee := g.sybilProtection.OnlineCommittee()
	onlineCommitteeTotalWeight := onlineCommittee.TotalWeight()
	blockWeightOnline := onlineCommittee.SelectAccounts(block.Witnesses()...).TotalWeight()

	fmt.Println("tryAcceptAndConfirm", block.ID(), "blockWeight", blockWeight, "blockWeightOnline", blockWeightOnline, "committeeTotalWeight", committeeTotalWeight, "onlineCommitteeTotalWeight", onlineCommitteeTotalWeight)

	if votes.IsThresholdReached(blockWeight, committeeTotalWeight, g.optsConfirmationThreshold) {
		fmt.Println("tryAcceptAndConfirm", block.ID(), "is confirmed")
		g.propagateAcceptanceAndConfirmation(block, true)
	} else if votes.IsThresholdReached(blockWeightOnline, onlineCommitteeTotalWeight, g.optsAcceptanceThreshold) {
		g.propagateAcceptanceAndConfirmation(block, false)
	}
}

func (g *Gadget) propagateAcceptanceAndConfirmation(initialBlock *blocks.Block, confirmed bool) {
	pastConeWalker := walker.New[iotago.BlockID](false).Push(initialBlock.ID())
	for pastConeWalker.HasNext() {
		blockID := pastConeWalker.Next()
		walkerBlock, exists := g.blockCache.Block(blockID)
		if !exists {
			panic(fmt.Sprintf("parent %s does not exist", blockID))
		}

		fmt.Println("propagateAcceptanceAndConfirmation", walkerBlock.ID(), "confirmed", confirmed)

		if walkerBlock.IsRootBlock() {
			continue
		}

		shouldWalkPastCone := false
		if !walkerBlock.IsAccepted() {
			g.acceptanceOrder.Queue(walkerBlock)
			shouldWalkPastCone = true
		}

		if confirmed && !walkerBlock.IsConfirmed() {
			g.markAsConfirmed(walkerBlock)
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
					g.acceptanceOrder.Queue(weakParent)
					if confirmed {
						g.markAsConfirmed(weakParent)
					}
				}
			}
		})
	}
}

func (g *Gadget) markAsAccepted(block *blocks.Block) (err error) {
	if block.SetAccepted() {
		g.events.BlockAccepted.Trigger(block)

		g.trackAcceptanceRatifierWeight(block)
	}

	return nil
}

func (g *Gadget) markAsConfirmed(block *blocks.Block) {
	if block.SetConfirmed() {
		g.events.BlockConfirmed.Trigger(block)

		g.trackConfirmationRatifierWeight(block)
	}
}

func (g *Gadget) acceptanceFailed(block *blocks.Block, err error) {
	panic(errors.Wrapf(err, "could not mark block %s as accepted", block.ID()))
}
