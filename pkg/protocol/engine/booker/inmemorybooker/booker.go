package inmemorybooker

import (
	"sync"

	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/core/causalorder"
	"github.com/iotaledger/hive.go/core/memstorage"
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
	// TODO: track votes
	// votePower := booker.NewBlockVotePower(block.ID(), block.IssuingTime())

	block.SetBooked()
	b.events.BlockBooked.Trigger(block)

	return nil
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
