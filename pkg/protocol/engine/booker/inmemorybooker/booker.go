package inmemorybooker

import (
	"fmt"

	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/core/causalorder"
	"github.com/iotaledger/hive.go/crypto/identity"
	"github.com/iotaledger/hive.go/ds/walker"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/booker"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/eviction"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Booker struct {
	events *booker.Events

	evictionState *eviction.State
	// validators    *sybilprotection.WeightedSet

	bookingOrder *causalorder.CausalOrder[iotago.SlotIndex, iotago.BlockID, *blocks.Block]

	workers *workerpool.Group

	blockCache         *blocks.Blocks
	setInvalidCallback func(*blocks.Block, error) bool

	module.Module
}

func NewProvider(opts ...options.Option[Booker]) module.Provider[*engine.Engine, booker.Booker] {
	return module.Provide(func(e *engine.Engine) booker.Booker {
		b := New(e.Workers.CreateGroup("Booker"), e.EvictionState, e.BlockCache, opts...)

		e.Events.BlockDAG.BlockSolid.Hook(func(block *blocks.Block) {
			if _, err := b.Queue(block); err != nil {
				b.events.Error.Trigger(err)
			}
		})

		e.HookConstructed(func() {
			b.events.Error.Hook(e.Events.Error.Trigger)
			e.Events.Booker.LinkTo(b.events)

			b.TriggerInitialized()
		})

		return b
	})
}

func New(workers *workerpool.Group, evictionState *eviction.State, blockCache *blocks.Blocks, opts ...options.Option[Booker]) *Booker {
	return options.Apply(&Booker{
		events:     booker.NewEvents(),
		blockCache: blockCache,

		evictionState: evictionState,
		workers:       workers,
	}, opts, func(b *Booker) {
		b.bookingOrder = causalorder.New(
			workers.CreatePool("BookingOrder", 2),
			blockCache.Block,
			(*blocks.Block).IsBooked,
			b.book,
			func(block *blocks.Block, _ error) { block.SetInvalid() },
			(*blocks.Block).Parents,
			causalorder.WithReferenceValidator[iotago.SlotIndex, iotago.BlockID](isReferenceValid),
		)

		b.evictionState.Events.SlotEvicted.Hook(b.evict)
	}, (*Booker).TriggerConstructed)
}

var _ booker.Booker = new(Booker)

// Queue checks if payload is solid and then adds the block to a Booker's CausalOrder.
func (b *Booker) Queue(block *blocks.Block) (wasQueued bool, err error) {
	if isSolid, err := b.isPayloadSolid(block); !isSolid {
		return false, errors.Wrap(err, "payload is not solid")
	}

	b.bookingOrder.Queue(block)
	return true, nil
}

func (b *Booker) Shutdown() {
	b.workers.Shutdown()
	b.TriggerStopped()
}

func (b *Booker) evict(slotIndex iotago.SlotIndex) {
	b.bookingOrder.EvictUntil(slotIndex)
}

func (b *Booker) isPayloadSolid(block *blocks.Block) (isPayloadSolid bool, err error) {
	return true, nil
}

func (b *Booker) book(block *blocks.Block) error {
	b.trackWitnessWeight(block)

	block.SetBooked()
	b.events.BlockBooked.Trigger(block)

	return nil
}

func (b *Booker) trackWitnessWeight(votingBlock *blocks.Block) {
	witness := identity.ID(votingBlock.Block().IssuerID)

	// TODO: only track witness weight for issuers having some weight: either they have cMana or they are in the committee.

	// Add the witness to the voting block itself as each block carries a vote for itself.
	votingBlock.AddWitness(witness)

	// Walk the block's past cone until we reach an accepted or already supported (by this witness) block.
	walk := walker.New[iotago.BlockID]().PushAll(votingBlock.Parents()...)
	for walk.HasNext() {
		blockID := walk.Next()
		block, exists := b.blockCache.Block(blockID)
		if !exists {
			panic(fmt.Sprintf("parent %s does not exist", blockID))
		}

		if block.IsRootBlock() {
			continue
		}

		// Skip further propagation if the witness is not new.
		if !block.AddWitness(witness) {
			continue
		}
		// TODO: Skip further propagation if block is already accepted.

		// TODO: We might not want to walk further than our direct weak and shallow liked parents.
		walk.PushAll(block.Parents()...)

		// TODO: here we might need to trigger an event WitnessAdded or something. However, doing this for each block might
		//  be a bit expensive. Instead, we could keep track of the lowest rank of blocks and only trigger for those and
		//  have a modified causal order in the acceptance gadget (with all booked blocks) that checks whether the acceptance threshold
		//  is reached for these lowest rank blocks and accordingly propagates acceptance to their children.
		b.events.WitnessAdded.Trigger(block)
	}
}

func (b *Booker) markInvalid(block *blocks.Block, reason error) {
	b.setInvalidCallback(block, errors.Wrap(reason, "block marked as invalid in Booker"))
}

// isReferenceValid checks if the reference between the child and its parent is valid.
func isReferenceValid(child *blocks.Block, parent *blocks.Block) (err error) {
	if parent.IsInvalid() {
		return errors.Errorf("parent %s of child %s is marked as invalid", parent.ID(), child.ID())
	}

	return nil
}
