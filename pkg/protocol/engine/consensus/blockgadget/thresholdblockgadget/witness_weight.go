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

func (g *Gadget) trackWitnessWeight(votingBlock *blocks.Block) {
	witness := votingBlock.Block().IssuerID

	// Only track witness weight for issuers that are part of the committee.
	if !g.sybilProtection.Committee().Has(witness) {
		return
	}

	// Add the witness to the voting block itself as each block carries a vote for itself.
	if votingBlock.AddWitness(witness) {
		g.tryPreAcceptAndPreConfirm(votingBlock)
		// return // TODO: do we need to continue here?
	}

	// Walk the block's past cone until we reach an accepted or already supported (by this witness) block.
	walk := walker.New[iotago.BlockID]().PushAll(votingBlock.Parents()...)
	for walk.HasNext() {
		blockID := walk.Next()
		block, exists := g.blockCache.Block(blockID)
		if !exists {
			panic(errors.Errorf("parent %s does not exist", blockID))
		}

		if block.IsRootBlock() {
			continue
		}

		// Skip propagation if the block is already accepted.
		if block.IsPreAccepted() {
			continue
		}

		// Skip further propagation if the witness is not new.
		if !block.AddWitness(witness) {
			continue
		}

		fmt.Println("trackWitnessWeight", votingBlock.ID(), "added", block.ID(), witness)

		block.ForEachParent(func(parent model.Parent) {
			switch parent.Type {
			case model.StrongParentType:
				walk.Push(parent.ID)
			case model.ShallowLikeParentType, model.WeakParentType:
				if weakParent, exists := g.blockCache.Block(parent.ID); exists {
					if weakParent.AddWitness(witness) {
						g.tryPreAcceptAndPreConfirm(block)
					}
				}
			}
		})

		// TODO: here we might need to trigger an event WitnessAdded or something. However, doing this for each block might
		//  be a bit expensive. Instead, we could keep track of the lowest rank of blocks and only trigger for those and
		//  have a modified causal order in the acceptance gadget (with all booked blocks) that checks whether the acceptance threshold
		//  is reached for these lowest rank blocks and accordingly propagates acceptance to their children.
		g.tryPreAcceptAndPreConfirm(block)
	}
}

func (g *Gadget) tryPreAcceptAndPreConfirm(block *blocks.Block) {
	committee := g.sybilProtection.Committee()
	committeeTotalWeight := committee.TotalWeight()
	blockWeight := committee.SelectAccounts(block.Witnesses()...).TotalWeight()

	onlineCommittee := g.sybilProtection.OnlineCommittee()
	onlineCommitteeTotalWeight := onlineCommittee.TotalWeight()
	blockWeightOnline := onlineCommittee.SelectAccounts(block.Witnesses()...).TotalWeight()

	fmt.Println("tryPreAcceptAndPreConfirm", block.ID(), "weight", blockWeight, "onlineWeight", blockWeightOnline, "committeeTotalWeight", committeeTotalWeight, "onlineCommitteeTotalWeight", onlineCommitteeTotalWeight)

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
			fmt.Println("propagatePreAcceptanceAndPreConfirmation", initialBlock.ID(), "preAccepted", walkerBlock.ID())
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
