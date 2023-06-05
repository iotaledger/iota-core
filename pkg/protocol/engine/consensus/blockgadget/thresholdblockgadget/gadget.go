package thresholdblockgadget

import (
	"fmt"

	"github.com/iotaledger/hive.go/core/causalorder"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/consensus/blockgadget"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/sybilprotection"
	iotago "github.com/iotaledger/iota.go/v4"
)

// region Gadget ///////////////////////////////////////////////////////////////////////////////////////////////////////

type Gadget struct {
	events *blockgadget.Events

	workers *workerpool.Group

	sybilProtection sybilprotection.SybilProtection
	blockCache      *blocks.Blocks

	acceptanceOrder           *causalorder.CausalOrder[iotago.SlotIndex, iotago.BlockID, *blocks.Block]
	ratifiedAcceptanceOrder   *causalorder.CausalOrder[iotago.SlotIndex, iotago.BlockID, *blocks.Block]
	confirmationOrder         *causalorder.CausalOrder[iotago.SlotIndex, iotago.BlockID, *blocks.Block]
	ratifiedConfirmationOrder *causalorder.CausalOrder[iotago.SlotIndex, iotago.BlockID, *blocks.Block]

	optsAcceptanceThreshold   float64
	optsConfirmationThreshold float64

	module.Module
}

func NewProvider(opts ...options.Option[Gadget]) module.Provider[*engine.Engine, blockgadget.Gadget] {
	return module.Provide(func(e *engine.Engine) blockgadget.Gadget {
		g := New(e.Workers.CreateGroup("BlockGadget"), e.BlockCache, e.SybilProtection, opts...)
		e.Events.Booker.WitnessAdded.Hook(g.tryAcceptAndConfirm)
		e.BlockCache.Evict.Hook(g.evictUntil)

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
			g.ratifiedConfirmationOrder = causalorder.New[iotago.SlotIndex, iotago.BlockID, *blocks.Block](g.workers.CreatePool("RatifiedConfirmationOrder", 2), blockCache.Block, (*blocks.Block).IsRatifiedConfirmed, g.markAsRatifiedConfirmed, g.ratifiedConfirmationFailed, (*blocks.Block).StrongParents)

			// TODO: introduce ratified confirmation order that does the same as ratified acceptance basically
			//  Also add following check: if block is already evicted: return it as confirmed if slot is finalized until then
			//  Come up with test scenario specifically for
			//		- acceptance and ratified acceptance (online committee weight is 50%)
			//		- confirmation and ratified confirmation (online committee weight is 100%)
		},
		(*Gadget).TriggerConstructed,
	)
}

func (g *Gadget) Events() *blockgadget.Events {
	return g.events
}

func (g *Gadget) Shutdown() {
	g.TriggerStopped()
	g.workers.Shutdown()
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

func (g *Gadget) IsBlockRatifiedConfirmed(blockID iotago.BlockID) bool {
	block, exists := g.blockCache.Block(blockID)
	return exists && block.IsRatifiedAccepted()
}

func (g *Gadget) evictUntil(index iotago.SlotIndex) {
	g.acceptanceOrder.EvictUntil(index)
	g.ratifiedAcceptanceOrder.EvictUntil(index)
	g.confirmationOrder.EvictUntil(index)
	g.ratifiedConfirmationOrder.EvictUntil(index)
	fmt.Println("evictUntil", index)
}

var _ blockgadget.Gadget = new(Gadget)
