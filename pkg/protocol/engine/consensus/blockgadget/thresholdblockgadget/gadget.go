package thresholdblockgadget

import (
	"fmt"

	"github.com/iotaledger/hive.go/ds/walker"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/consensus/blockgadget"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/sybilprotection"
	iotago "github.com/iotaledger/iota.go/v4"
)

// region Gadget ///////////////////////////////////////////////////////////////////////////////////////////////////////

type Gadget struct {
	events *blockgadget.Events

	sybilProtection sybilprotection.SybilProtection
	blockCache      *blocks.Blocks

	optsAcceptanceThreshold               float64
	optsConfirmationThreshold             float64
	optsConfirmationRatificationThreshold iotago.SlotIndex

	module.Module
}

func NewProvider(opts ...options.Option[Gadget]) module.Provider[*engine.Engine, blockgadget.Gadget] {
	return module.Provide(func(e *engine.Engine) blockgadget.Gadget {
		g := New(e.BlockCache, e.SybilProtection, opts...)
		e.Events.Booker.BlockBooked.Hook(g.trackWitnessWeight)

		e.Events.BlockGadget.LinkTo(g.events)

		return g
	})
}

func New(blockCache *blocks.Blocks, sybilProtection sybilprotection.SybilProtection, opts ...options.Option[Gadget]) *Gadget {
	return options.Apply(&Gadget{
		events:          blockgadget.NewEvents(),
		sybilProtection: sybilProtection,
		blockCache:      blockCache,

		optsAcceptanceThreshold:               0.67,
		optsConfirmationThreshold:             0.67,
		optsConfirmationRatificationThreshold: 2,
	}, opts,
		(*Gadget).TriggerConstructed,
	)
}

func (g *Gadget) Events() *blockgadget.Events {
	return g.events
}

func (g *Gadget) Shutdown() {
	g.TriggerStopped()
}

func (g *Gadget) propagate(initialBlockIDs iotago.BlockIDs, evaluateFunc func(block *blocks.Block) bool) {
	walk := walker.New[iotago.BlockID](false).PushAll(initialBlockIDs...)
	for walk.HasNext() {
		blockID := walk.Next()
		block, exists := g.blockCache.Block(blockID)
		if !exists {
			panic(fmt.Sprintf("parent %s does not exist", blockID))
		}

		if block.IsRootBlock() {
			continue
		}

		if !evaluateFunc(block) {
			continue
		}

		walk.PushAll(block.Parents()...)
	}
}
