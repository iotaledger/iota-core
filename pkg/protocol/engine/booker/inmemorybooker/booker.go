package inmemorybooker

import (
	"fmt"

	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/core/causalorder"
	"github.com/iotaledger/hive.go/ds/walker"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/booker"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/sybilprotection"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Booker struct {
	events *booker.Events

	sybilProtection sybilprotection.SybilProtection

	bookingOrder *causalorder.CausalOrder[iotago.SlotIndex, iotago.BlockID, *blocks.Block]

	workers *workerpool.Group

	blockCache *blocks.Blocks

	module.Module
}

func NewProvider(opts ...options.Option[Booker]) module.Provider[*engine.Engine, booker.Booker] {
	return module.Provide(func(e *engine.Engine) booker.Booker {
		b := New(e.Workers.CreateGroup("Booker"), e.SybilProtection, e.BlockCache, opts...)

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

func New(workers *workerpool.Group, sybilProtection sybilprotection.SybilProtection, blockCache *blocks.Blocks, opts ...options.Option[Booker]) *Booker {
	return options.Apply(&Booker{
		events:          booker.NewEvents(),
		sybilProtection: sybilProtection,
		blockCache:      blockCache,

		workers: workers,
	}, opts, func(b *Booker) {
		b.bookingOrder = causalorder.New(
			workers.CreatePool("BookingOrder", 2),
			blockCache.Block,
			(*blocks.Block).IsBooked,
			b.book,
			b.markInvalid,
			(*blocks.Block).Parents,
			causalorder.WithReferenceValidator[iotago.SlotIndex, iotago.BlockID](isReferenceValid),
		)

		blockCache.Evict.Hook(b.evict)
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

func (b *Booker) isPayloadSolid(_ *blocks.Block) (isPayloadSolid bool, err error) {
	return true, nil
}

func (b *Booker) book(block *blocks.Block) error {
	block.SetBooked()
	b.events.BlockBooked.Trigger(block)

	b.trackWitnessWeight(block)

	return nil
}

func (b *Booker) trackWitnessWeight(votingBlock *blocks.Block) {
	witness := votingBlock.Block().IssuerID

	// Only track witness weight for issuers that are part of the committee.
	if !b.sybilProtection.Committee().Has(witness) {
		return
	}

	// Add the witness to the voting block itself as each block carries a vote for itself.
	if votingBlock.AddWitness(witness) {
		b.events.WitnessAdded.Trigger(votingBlock)
	}

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

		// Skip propagation if the block is already accepted.
		if block.IsAccepted() {
			continue
		}

		// Skip further propagation if the witness is not new.
		if !block.AddWitness(witness) {
			continue
		}

		block.ForEachParent(func(parent model.Parent) {
			switch parent.Type {
			case model.StrongParentType:
				walk.Push(parent.ID)
			case model.ShallowLikeParentType, model.WeakParentType:
				if weakParent, exists := b.blockCache.Block(parent.ID); exists {
					if weakParent.AddWitness(witness) {
						b.events.WitnessAdded.Trigger(weakParent)
					}
				}
			}
		})

		// TODO: here we might need to trigger an event WitnessAdded or something. However, doing this for each block might
		//  be a bit expensive. Instead, we could keep track of the lowest rank of blocks and only trigger for those and
		//  have a modified causal order in the acceptance gadget (with all booked blocks) that checks whether the acceptance threshold
		//  is reached for these lowest rank blocks and accordingly propagates acceptance to their children.
		b.events.WitnessAdded.Trigger(block)
	}
}

func (b *Booker) markInvalid(block *blocks.Block, _ error) {
	block.SetInvalid()
}

// isReferenceValid checks if the reference between the child and its parent is valid.
func isReferenceValid(child *blocks.Block, parent *blocks.Block) (err error) {
	if parent.IsInvalid() {
		return errors.Errorf("parent %s of child %s is marked as invalid", parent.ID(), child.ID())
	}

	return nil
}
