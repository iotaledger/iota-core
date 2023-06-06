package thresholdblockgadget

import (
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

	acceptanceOrder         *causalorder.CausalOrder[iotago.SlotIndex, iotago.BlockID, *blocks.Block]
	ratifiedAcceptanceOrder *causalorder.CausalOrder[iotago.SlotIndex, iotago.BlockID, *blocks.Block]

	optsAcceptanceThreshold               float64
	optsConfirmationThreshold             float64
	optsConfirmationRatificationThreshold iotago.SlotIndex

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

		optsAcceptanceThreshold:               0.67,
		optsConfirmationThreshold:             0.67,
		optsConfirmationRatificationThreshold: 2,
	}, opts,
		func(g *Gadget) {
			g.acceptanceOrder = causalorder.New[iotago.SlotIndex, iotago.BlockID, *blocks.Block](g.workers.CreatePool("AcceptanceOrder", 2), blockCache.Block, (*blocks.Block).IsAccepted, g.markAsAccepted, g.acceptanceFailed, (*blocks.Block).StrongParents)
			g.ratifiedAcceptanceOrder = causalorder.New[iotago.SlotIndex, iotago.BlockID, *blocks.Block](g.workers.CreatePool("RatifiedAcceptanceOrder", 2), blockCache.Block, (*blocks.Block).IsRatifiedAccepted, g.markAsRatifiedAccepted, g.ratifiedAcceptanceFailed, (*blocks.Block).StrongParents)
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
}

var _ blockgadget.Gadget = new(Gadget)
