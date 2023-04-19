package thresholdblockgadget

import (
	"fmt"

	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/core/causalorder"
	"github.com/iotaledger/hive.go/crypto/identity"
	"github.com/iotaledger/hive.go/ds/walker"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/consensus/blockgadget"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/sybilprotection"
	"github.com/iotaledger/iota-core/pkg/votes"
	iotago "github.com/iotaledger/iota.go/v4"
)

// region Gadget ///////////////////////////////////////////////////////////////////////////////////////////////////////

type Gadget struct {
	events *blockgadget.Events

	workers *workerpool.Group

	sybilProtection sybilprotection.SybilProtection
	blockCache      *blocks.Blocks

	acceptanceOrder         *causalorder.CausalOrder[iotago.SlotIndex, iotago.BlockID, *blocks.Block]
	ratifiedAcceptanceOrder *causalorder.CausalOrder[iotago.SlotIndex, iotago.BlockID, *blocks.Block]
	confirmationOrder       *causalorder.CausalOrder[iotago.SlotIndex, iotago.BlockID, *blocks.Block]

	optsAcceptanceThreshold   float64
	optsConfirmationThreshold float64

	module.Module
}

func NewProvider(opts ...options.Option[Gadget]) module.Provider[*engine.Engine, blockgadget.Gadget] {
	return module.Provide(func(e *engine.Engine) blockgadget.Gadget {
		g := New(e.Workers.CreateGroup("BlockGadget"), e.BlockCache, e.SybilProtection, opts...)
		e.Events.Booker.WitnessAdded.Hook(g.tryAccept)

		e.Events.BlockGadget.LinkTo(g.events)

		return g
	})
}

func New(workers *workerpool.Group, blockCache *blocks.Blocks, sybilProtection sybilprotection.SybilProtection, opts ...options.Option[Gadget]) *Gadget {
	return options.Apply(&Gadget{
		events:          blockgadget.NewEvents(),
		workers:         workers,
		sybilProtection: sybilProtection,
		blockCache:      blockCache,

		optsAcceptanceThreshold:   0.67,
		optsConfirmationThreshold: 0.67,
	}, opts,
		func(g *Gadget) {
			g.acceptanceOrder = causalorder.New[iotago.SlotIndex, iotago.BlockID, *blocks.Block](g.workers.CreatePool("AcceptanceOrder", 2), blockCache.Block, (*blocks.Block).IsAccepted, g.markAsAccepted, g.acceptanceFailed, (*blocks.Block).StrongParents)
			g.ratifiedAcceptanceOrder = causalorder.New[iotago.SlotIndex, iotago.BlockID, *blocks.Block](g.workers.CreatePool("RatifiedAcceptanceOrder", 2), blockCache.Block, (*blocks.Block).IsRatifiedAccepted, g.markAsRatifiedAccepted, g.ratifiedAcceptanceFailed, (*blocks.Block).StrongParents)
			g.confirmationOrder = causalorder.New[iotago.SlotIndex, iotago.BlockID, *blocks.Block](g.workers.CreatePool("ConfirmationOrder", 2), blockCache.Block, (*blocks.Block).IsConfirmed, g.markAsConfirmed, g.confirmationFailed, (*blocks.Block).StrongParents)

			// TODO: revisit whole eviction
			// g.evictionState.Events.SlotEvicted.Hook(g.EvictUntil, event.WithWorkerPool(g.workers.CreatePool("Eviction", 1)))
		},
		(*Gadget).TriggerConstructed,
	)
}

func (g *Gadget) Events() *blockgadget.Events {
	return g.events
}

func (g *Gadget) Shutdown() {
	g.workers.Shutdown()
	g.TriggerStopped()
}

// IsBlockAccepted returns whether the given block is accepted.
func (g *Gadget) IsBlockAccepted(blockID iotago.BlockID) (accepted bool) {
	block, exists := g.blockCache.Block(blockID)
	return exists && block.IsAccepted()
}

// IsBlockRatifiedAccepted returns whether the given block is ratified accepted.
func (g *Gadget) IsBlockRatifiedAccepted(blockID iotago.BlockID) (accepted bool) {
	block, exists := g.blockCache.Block(blockID)
	return exists && block.IsRatifiedAccepted()
}

func (g *Gadget) IsBlockConfirmed(blockID iotago.BlockID) bool {
	block, exists := g.blockCache.Block(blockID)
	return exists && block.IsConfirmed()
}

func (g *Gadget) tryAccept(block *blocks.Block) {
	committee := g.sybilProtection.Committee()
	blockWeight := committee.SelectAccounts(block.Witnesses()...).TotalWeight()
	onlineCommitteeTotalWeight := g.sybilProtection.OnlineCommittee().TotalWeight()

	if votes.IsThresholdReached(blockWeight, onlineCommitteeTotalWeight, g.optsAcceptanceThreshold) {
		g.propagateAcceptance(block)
	}
}

func (g *Gadget) trackRatifierWeight(votingBlock *blocks.Block) {
	ratifier := identity.ID(votingBlock.Block().IssuerID)

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
		if !block.AddRatifier(ratifier) {
			continue
		}

		block.ForEachParent(func(parent model.Parent) {
			switch parent.Type {
			case model.StrongParentType:
				walk.Push(parent.ID)
			case model.ShallowLikeParentType, model.WeakParentType:
				if weakParent, exists := g.blockCache.Block(parent.ID); exists {
					if weakParent.AddRatifier(ratifier) {
						g.tryRatifiedAcceptAndConfirm(weakParent)
					}
				}
			}
		})

		g.tryRatifiedAcceptAndConfirm(block)
	}
}

// tryRatifiedAcceptAndConfirm checks if there is enough active ratified weight to confirm blocks and then checks
// if the marker has accumulated enough witness weight to be both accepted and confirmed.
// Acceptance and Confirmation use the same threshold if confirmation is possible.
// If there is not enough online weight to achieve confirmation, then acceptance condition is evaluated based on total active weight.
func (g *Gadget) tryRatifiedAcceptAndConfirm(block *blocks.Block) {
	committee := g.sybilProtection.Committee()
	committeeTotalWeight := committee.TotalWeight()

	blockWeight := committee.SelectAccounts(block.Ratifiers()...).TotalWeight()
	onlineCommitteeTotalWeight := g.sybilProtection.OnlineCommittee().TotalWeight()

	// check if we reached the confirmation threshold based on the total commitee weight
	if votes.IsThresholdReached(blockWeight, committeeTotalWeight, g.optsConfirmationThreshold) {
		g.propagateRatifiedAcceptanceAndConfirmation(block, true)
	} else if votes.IsThresholdReached(blockWeight, onlineCommitteeTotalWeight, g.optsAcceptanceThreshold) {
		g.propagateRatifiedAcceptanceAndConfirmation(block, false)
	}
}

func (g *Gadget) EvictUntil(index iotago.SlotIndex) {
	g.acceptanceOrder.EvictUntil(index)
	g.confirmationOrder.EvictUntil(index)
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

		shouldWalkPastCone := false
		if !walkerBlock.IsAccepted() {
			g.acceptanceOrder.Queue(walkerBlock)
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
				}
			}
		})
	}
}

func (g *Gadget) propagateRatifiedAcceptanceAndConfirmation(initialBlock *blocks.Block, confirmed bool) {
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
		if !walkerBlock.IsRatifiedAccepted() {
			g.ratifiedAcceptanceOrder.Queue(walkerBlock)
			shouldWalkPastCone = true
		}

		if confirmed && !walkerBlock.IsConfirmed() {
			g.confirmationOrder.Queue(walkerBlock)
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
					g.ratifiedAcceptanceOrder.Queue(weakParent)
					if confirmed {
						g.confirmationOrder.Queue(weakParent)
					}
				}
			}
		})
	}
}

func (g *Gadget) markAsAccepted(block *blocks.Block) (err error) {
	if block.SetAccepted() {
		g.events.BlockAccepted.Trigger(block)

		g.trackRatifierWeight(block)

		// TODO: set inclusion state of TX if block contains one.
		// set ConfirmationState of payload (applicable only to transactions)
		// if tx, ok := block.Transaction(); ok {
		// 	g.memPool.SetTransactionInclusionSlot(tx.ID(), g.slotTimeProvider.IndexFromTime(block.IssuingTime()))
		// }
	}

	return nil
}

func (g *Gadget) markAsRatifiedAccepted(block *blocks.Block) (err error) {
	if block.SetRatifiedAccepted() {
		g.events.BlockRatifiedAccepted.Trigger(block)
	}

	return nil
}

func (g *Gadget) markAsConfirmed(block *blocks.Block) (err error) {
	if block.SetConfirmed() {
		g.events.BlockConfirmed.Trigger(block)
	}

	return nil
}

func (g *Gadget) acceptanceFailed(block *blocks.Block, err error) {
	panic(errors.Wrapf(err, "could not mark block %s as accepted", block.ID()))
}

func (g *Gadget) ratifiedAcceptanceFailed(block *blocks.Block, err error) {
	panic(errors.Wrapf(err, "could not mark block %s as ratified accepted", block.ID()))
}

func (g *Gadget) confirmationFailed(block *blocks.Block, err error) {
	panic(errors.Wrapf(err, "could not mark block %s as confirmed", block.ID()))
}

var _ blockgadget.Gadget = new(Gadget)
