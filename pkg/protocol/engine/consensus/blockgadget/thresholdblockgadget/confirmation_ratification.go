package thresholdblockgadget

import (
	"fmt"

	"github.com/iotaledger/hive.go/ds/walker"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/votes"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (g *Gadget) trackConfirmationRatifierWeight(ratifierBlock *blocks.Block) {
	ratifier := ratifierBlock.Block().IssuerID
	ratifierBlockIndex := ratifierBlock.ID().Index()

	// Only track ratifier weight for issuers that are part of the committee.
	if !g.sybilProtection.Committee().Has(ratifier) {
		return
	}

	walk := walker.New[iotago.BlockID]().PushAll(ratifierBlock.Parents()...)
	for walk.HasNext() {
		blockID := walk.Next()

		fmt.Println("trackConfirmationRatifierWeight", ratifierBlock.ID(), blockID)
		if blockID.Index() <= ratifierBlockIndex-g.optsConfirmationRatificationThreshold {
			fmt.Println("trackConfirmationRatifierWeight", ratifierBlock.ID(), blockID, "skipped")
			continue
		}

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
		if block.SetRatifiedConfirmed() {
			g.events.BlockRatifiedConfirmed.Trigger(block)
		}
	}
}
