package inmemorybooker

import (
	"fmt"
	"sync"

	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/core/causalorder"
	"github.com/iotaledger/hive.go/core/memstorage"
	"github.com/iotaledger/hive.go/crypto/identity"
	"github.com/iotaledger/hive.go/ds/walker"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blockdag"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/booker"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/eviction"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Booker struct {
	events *booker.Events

	evictionState *eviction.State
	// validators    *sybilprotection.WeightedSet

	bookingOrder  *causalorder.CausalOrder[iotago.SlotIndex, iotago.BlockID, *booker.Block]
	blocks        *memstorage.IndexedStorage[iotago.SlotIndex, iotago.BlockID, *booker.Block]
	evictionMutex sync.RWMutex

	workers *workerpool.Group

	rootBlockProvider  func(iotago.BlockID) (*blockdag.Block, bool)
	setInvalidCallback func(*blockdag.Block, error) bool

	module.Module
}

func NewProvider(opts ...options.Option[Booker]) module.Provider[*engine.Engine, booker.Booker] {
	return module.Provide(func(e *engine.Engine) booker.Booker {
		b := New(e.Workers.CreateGroup("Booker"), e.EvictionState, opts...)

		e.Events.BlockDAG.BlockSolid.Hook(func(block *blockdag.Block) {
			if _, err := b.Queue(booker.NewBlock(block)); err != nil {
				b.events.Error.Trigger(err)
			}
		})

		e.HookConstructed(func() {
			b.events.Error.Hook(e.Events.Error.Trigger)
			e.Events.Booker.LinkTo(b.events)

			b.Initialize(e.RootBlock, e.BlockDAG.SetInvalid)
		})

		return b
	})
}

func New(workers *workerpool.Group, evictionState *eviction.State, opts ...options.Option[Booker]) *Booker {
	return options.Apply(&Booker{
		events: booker.NewEvents(),
		blocks: memstorage.NewIndexedStorage[iotago.SlotIndex, iotago.BlockID, *booker.Block](),

		evictionState: evictionState,
		workers:       workers,
	}, opts, func(b *Booker) {
		b.bookingOrder = causalorder.New[iotago.SlotIndex](
			workers.CreatePool("BookingOrder", 2),
			b.Block,
			(*booker.Block).IsBooked,
			b.book,
			b.markInvalid,
			(*booker.Block).Parents,
			causalorder.WithReferenceValidator[iotago.SlotIndex, iotago.BlockID](isReferenceValid),
		)

		b.evictionState.Events.SlotEvicted.Hook(b.evict)

	}, (*Booker).TriggerConstructed)
}

func (b *Booker) Initialize(rootBlockProvider func(iotago.BlockID) (*blockdag.Block, bool), setInvalidCallback func(*blockdag.Block, error) bool) {
	b.rootBlockProvider = rootBlockProvider
	b.setInvalidCallback = setInvalidCallback

	b.TriggerInitialized()
}

var _ booker.Booker = new(Booker)

// Queue checks if payload is solid and then adds the block to a Booker's CausalOrder.
func (b *Booker) Queue(block *booker.Block) (wasQueued bool, err error) {
	if wasQueued, err = b.queue(block); wasQueued {
		b.bookingOrder.Queue(block)
		return true, nil
	}

	return false, err
}

func (b *Booker) queue(block *booker.Block) (ready bool, err error) {
	b.evictionMutex.RLock()
	defer b.evictionMutex.RUnlock()

	if b.evictionState.InEvictedSlot(block.ID()) {
		return false, errors.Errorf("%s is too old (issued at: %s)", block.ID(), block.Block.Block().IssuingTime)
	}

	b.blocks.Get(block.ID().Index(), true).Set(block.ID(), block)

	return b.isPayloadSolid(block)
}

// Block retrieves a Block with metadata from the in-memory storage of the Booker.
func (b *Booker) Block(id iotago.BlockID) (block *booker.Block, exists bool) {
	b.evictionMutex.RLock()
	defer b.evictionMutex.RUnlock()

	return b.block(id)
}

func (b *Booker) evict(slotIndex iotago.SlotIndex) {
	b.bookingOrder.EvictUntil(slotIndex)

	b.evictionMutex.Lock()
	defer b.evictionMutex.Unlock()

	b.blocks.Evict(slotIndex)
}

func (b *Booker) isPayloadSolid(block *booker.Block) (isPayloadSolid bool, err error) {
	return true, nil
}

// block retrieves the Block with given id from the mem-storage.
func (b *Booker) block(id iotago.BlockID) (block *booker.Block, exists bool) {
	if rootBlock, isRootBlock := b.rootBlockProvider(id); isRootBlock {
		return booker.NewRootBlock(rootBlock), true
	}

	storage := b.blocks.Get(id.Index(), false)
	if storage == nil {
		return nil, false
	}

	return storage.Get(id)
}

func (b *Booker) book(block *booker.Block) error {
	b.trackWitnessWeight(block)

	block.SetBooked()
	b.events.BlockBooked.Trigger(block)

	return nil
}

func (b *Booker) trackWitnessWeight(votingBlock *booker.Block) {
	witness := identity.ID(votingBlock.ModelsBlock.Block().IssuerID)

	// TODO: only track witness weight for current validators.

	// Add the witness to the voting block itself as each block carries a vote for itself.
	votingBlock.AddWitness(witness)

	// Walk the block's past cone until we reach an accepted or already supported (by this witness) block.
	walk := walker.New[iotago.BlockID]().PushAll(votingBlock.Parents()...)
	for walk.HasNext() {
		blockID := walk.Next()
		block, exists := b.block(blockID)
		if !exists {
			panic(fmt.Sprintf("parent %s does not exist", blockID))
		}

		// Skip further propagation if the witness is not new.
		if !block.AddWitness(witness) {
			continue
		}
		// TODO: Skip further propagation if block is already accepted.

		walk.PushAll(block.Parents()...)

		// TODO: here we might need to trigger an event WitnessAdded or something. However, doing this for each block might
		//  be a bit expensive. Instead, we could keep track of the lowest rank of blocks and only trigger for those and
		//  have a modified causal order in the acceptance gadget (with all booked blocks) that checks whether the acceptance threshold
		//  is reached for these lowest rank blocks and accordingly propagates acceptance to their children.
	}
}

func (b *Booker) markInvalid(block *booker.Block, reason error) {
	b.setInvalidCallback(block.Block, errors.Wrap(reason, "block marked as invalid in Booker"))
}

// isReferenceValid checks if the reference between the child and its parent is valid.
func isReferenceValid(child *booker.Block, parent *booker.Block) (err error) {
	if parent.IsInvalid() {
		return errors.Errorf("parent %s of child %s is marked as invalid", parent.ID(), child.ID())
	}

	return nil
}
