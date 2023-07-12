package thresholdblockgadget

import (
	"fmt"

	"github.com/iotaledger/hive.go/ds/set"
	"github.com/iotaledger/hive.go/ds/walker"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/consensus/blockgadget"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection/seatmanager"
	iotago "github.com/iotaledger/iota.go/v4"
)

// region Gadget ///////////////////////////////////////////////////////////////////////////////////////////////////////

type Gadget struct {
	events *blockgadget.Events

	seatManager seatmanager.SeatManager
	blockCache  *blocks.Blocks

	optsAcceptanceThreshold               float64
	optsConfirmationThreshold             float64
	optsConfirmationRatificationThreshold iotago.SlotIndex

	module.Module
}

func NewProvider(opts ...options.Option[Gadget]) module.Provider[*engine.Engine, blockgadget.Gadget] {
	return module.Provide(func(e *engine.Engine) blockgadget.Gadget {
		g := New(e.BlockCache, e.SybilProtection.SeatManager(), opts...)

		wp := e.Workers.CreatePool("ThresholdBlockGadget", 1)
		e.Events.Booker.BlockBooked.Hook(g.TrackWitnessWeight, event.WithWorkerPool(wp))

		e.Events.BlockGadget.LinkTo(g.events)

		return g
	})
}

func New(blockCache *blocks.Blocks, seatManager seatmanager.SeatManager, opts ...options.Option[Gadget]) *Gadget {
	return options.Apply(&Gadget{
		events:      blockgadget.NewEvents(),
		seatManager: seatManager,
		blockCache:  blockCache,

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

// propagate performs a breadth-first past cone walk starting at initialBlockIDs. evaluateFunc is called for every block visited
// and needs to return whether to continue the walk further.
func (g *Gadget) propagate(initialBlockIDs iotago.BlockIDs, evaluateFunc func(block *blocks.Block) bool) {
	walk := walker.New[iotago.BlockID](false).PushAll(initialBlockIDs...)
	for walk.HasNext() {
		blockID := walk.Next()
		block, exists := g.blockCache.Block(blockID)

		// If the block doesn't exist is either in the process of being evicted (accepted or orphaned), or we should
		// find it as a root block.
		if !exists || block.IsRootBlock() {
			continue
		}

		if !block.IsBooked() {
			panic(fmt.Sprintf("block %s is not booked", blockID))
		}

		if !evaluateFunc(block) {
			continue
		}

		walk.PushAll(block.Parents()...)
	}
}

func anyChildInSet(block *blocks.Block, set set.Set[iotago.BlockID]) bool {
	for _, child := range block.Children() {
		if set.Has(child.ID()) {
			return true
		}
	}

	return false
}
