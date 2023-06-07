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

	preAcceptanceOrder *causalorder.CausalOrder[iotago.SlotIndex, iotago.BlockID, *blocks.Block]
	acceptanceOrder    *causalorder.CausalOrder[iotago.SlotIndex, iotago.BlockID, *blocks.Block]

	optsAcceptanceThreshold               float64
	optsConfirmationThreshold             float64
	optsConfirmationRatificationThreshold iotago.SlotIndex

	module.Module
}

func NewProvider(opts ...options.Option[Gadget]) module.Provider[*engine.Engine, blockgadget.Gadget] {
	return module.Provide(func(e *engine.Engine) blockgadget.Gadget {
		g := New(e.Workers.CreateGroup("BlockGadget"), e.BlockCache, e.SybilProtection, opts...)
		e.Events.Booker.BlockBooked.Hook(g.trackWitnessWeight)
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
			g.preAcceptanceOrder = causalorder.New[iotago.SlotIndex, iotago.BlockID, *blocks.Block](g.workers.CreatePool("PreAcceptanceOrder", 2), blockCache.Block, (*blocks.Block).IsPreAccepted, g.markAsPreAccepted, g.preAcceptanceFailed, (*blocks.Block).StrongParents)
			g.acceptanceOrder = causalorder.New[iotago.SlotIndex, iotago.BlockID, *blocks.Block](g.workers.CreatePool("AcceptanceOrder", 2), blockCache.Block, (*blocks.Block).IsAccepted, g.markAsAccepted, g.acceptanceFailed, (*blocks.Block).StrongParents)
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

func (g *Gadget) evictUntil(index iotago.SlotIndex) {
	g.preAcceptanceOrder.EvictUntil(index)
	g.acceptanceOrder.EvictUntil(index)
}

var _ blockgadget.Gadget = new(Gadget)
