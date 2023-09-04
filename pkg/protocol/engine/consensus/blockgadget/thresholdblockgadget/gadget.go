package thresholdblockgadget

import (
	"fmt"

	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/ds/walker"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/core/account"
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

	lastAcceptedBlockIndex  reactive.Variable[iotago.SlotIndex]
	lastConfirmedBlockIndex reactive.Variable[iotago.SlotIndex]

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

		lastAcceptedBlockIndex:  reactive.NewVariable[iotago.SlotIndex](),
		lastConfirmedBlockIndex: reactive.NewVariable[iotago.SlotIndex](),

		optsAcceptanceThreshold:               0.67,
		optsConfirmationThreshold:             0.67,
		optsConfirmationRatificationThreshold: 2,
	}, opts, func(g *Gadget) {
		g.events.BlockAccepted.Hook(func(block *blocks.Block) {
			g.lastAcceptedBlockIndex.Compute(func(lastAcceptedBlockIndex iotago.SlotIndex) iotago.SlotIndex {
				if block.ID().Index() < lastAcceptedBlockIndex {
					return lastAcceptedBlockIndex
				}

				return block.ID().Index()
			})
		})

		g.events.BlockConfirmed.Hook(func(block *blocks.Block) {
			g.lastConfirmedBlockIndex.Compute(func(lastConfirmedBlockIndex iotago.SlotIndex) iotago.SlotIndex {
				if block.ID().Index() < lastConfirmedBlockIndex {
					return lastConfirmedBlockIndex
				}

				return block.ID().Index()
			})
		})
	}, (*Gadget).TriggerConstructed)
}

func (g *Gadget) Events() *blockgadget.Events {
	return g.events
}

func (g *Gadget) LastAcceptedBlockIndex() iotago.SlotIndex {
	return g.lastAcceptedBlockIndex.Get()
}

func (g *Gadget) LastAcceptedBlockIndexR() reactive.Variable[iotago.SlotIndex] {
	return g.lastAcceptedBlockIndex
}

func (g *Gadget) LastConfirmedBlockIndex() iotago.SlotIndex {
	return g.lastConfirmedBlockIndex.Get()
}

func (g *Gadget) LastConfirmedBlockIndexR() reactive.Variable[iotago.SlotIndex] {
	return g.lastConfirmedBlockIndex
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

		// If the block doesn't exist it is either in the process of being evicted (accepted or orphaned), or we should
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

func (g *Gadget) isCommitteeValidationBlock(block *blocks.Block) (seat account.SeatIndex, isValid bool) {
	if _, isValidationBlock := block.ValidationBlock(); !isValidationBlock {
		return 0, false
	}

	// Only accept blocks for issuers that are part of the committee.
	return g.seatManager.Committee(block.ID().Index()).GetSeat(block.ProtocolBlock().IssuerID)
}

func anyChildInSet(block *blocks.Block, set ds.Set[iotago.BlockID]) bool {
	for _, child := range block.Children() {
		if set.Has(child.ID()) {
			return true
		}
	}

	return false
}
