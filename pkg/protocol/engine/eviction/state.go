package eviction

import (
	"io"
	"math"
	"sync"

	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/core/memstorage"
	"github.com/iotaledger/hive.go/ds/ringbuffer"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/storage"
	"github.com/iotaledger/iota-core/pkg/stream"
	iotago "github.com/iotaledger/iota.go/v4"
)

// State represents the state of the eviction and keeps track of the root blocks.
type State struct {
	Events *Events

	rootBlocks       *memstorage.IndexedStorage[iotago.SlotIndex, iotago.BlockID, iotago.CommitmentID]
	latestRootBlocks *ringbuffer.RingBuffer[iotago.BlockID]
	storage          *storage.Storage
	lastEvictedSlot  iotago.SlotIndex
	evictionMutex    sync.RWMutex
	triggerMutex     sync.Mutex

	optsRootBlocksEvictionDelay iotago.SlotIndex
}

// NewState creates a new eviction State.
func NewState(storageInstance *storage.Storage, opts ...options.Option[State]) (state *State) {
	return options.Apply(&State{
		Events:                      NewEvents(),
		rootBlocks:                  memstorage.NewIndexedStorage[iotago.SlotIndex, iotago.BlockID, iotago.CommitmentID](),
		latestRootBlocks:            ringbuffer.NewRingBuffer[iotago.BlockID](8),
		storage:                     storageInstance,
		lastEvictedSlot:             storageInstance.Settings.LatestCommitment().Index,
		optsRootBlocksEvictionDelay: 3,
	}, opts)
}

// EvictUntil triggers the SlotEvicted event for every evicted slot and evicts all root blocks until the delayed
// root blocks eviction threshold.
func (s *State) EvictUntil(index iotago.SlotIndex) {
	s.triggerMutex.Lock()
	defer s.triggerMutex.Unlock()

	s.evictionMutex.Lock()

	lastEvictedSlot := s.lastEvictedSlot
	if index <= lastEvictedSlot {
		s.evictionMutex.Unlock()
		return
	}

	for currentIndex := lastEvictedSlot; currentIndex < index; currentIndex++ {
		if delayedIndex := s.delayedBlockEvictionThreshold(currentIndex); delayedIndex >= 0 {
			s.rootBlocks.Evict(delayedIndex)
		}
	}
	s.lastEvictedSlot = index
	s.evictionMutex.Unlock()

	for currentIndex := lastEvictedSlot + 1; currentIndex <= index; currentIndex++ {
		s.Events.SlotEvicted.Trigger(currentIndex)
	}
}

// LastEvictedSlot returns the last evicted slot.
func (s *State) LastEvictedSlot() iotago.SlotIndex {
	s.evictionMutex.RLock()
	defer s.evictionMutex.RUnlock()

	return s.lastEvictedSlot
}

// EarliestRootCommitmentID returns the earliest commitment that rootblocks are committing to across all rootblocks.
func (s *State) EarliestRootCommitmentID() (earliestCommitment iotago.CommitmentID) {
	earliestCommitment = iotago.NewSlotIdentifier(math.MaxInt64, [32]byte{})

	s.rootBlocks.ForEach(func(index iotago.SlotIndex, storage *shrinkingmap.ShrinkingMap[iotago.BlockID, iotago.CommitmentID]) {
		storage.ForEach(func(id iotago.BlockID, commitmentID iotago.CommitmentID) bool {
			if commitmentID.Index() < earliestCommitment.Index() {
				earliestCommitment = commitmentID
			}

			return true
		})
	})

	if earliestCommitment.Index() == math.MaxInt64 {
		return iotago.NewEmptyCommitment().MustID()
	}

	return earliestCommitment
}

// InEvictedSlot checks if the Block associated with the given id is too old (in a pruned slot).
func (s *State) InEvictedSlot(id iotago.BlockID) bool {
	s.evictionMutex.RLock()
	defer s.evictionMutex.RUnlock()

	return id.Index() <= s.lastEvictedSlot
}

// AddRootBlock inserts a solid entry point to the seps map.
func (s *State) AddRootBlock(id iotago.BlockID, commitmentID iotago.CommitmentID) {
	s.evictionMutex.RLock()
	defer s.evictionMutex.RUnlock()

	if id.Index() <= s.delayedBlockEvictionThreshold(s.lastEvictedSlot) {
		return
	}

	if s.rootBlocks.Get(id.Index(), true).Set(id, commitmentID) {
		if err := s.storage.RootBlocks.Store(id, commitmentID); err != nil {
			panic(errors.Wrapf(err, "failed to store root block %s", id))
		}
	}

	s.latestRootBlocks.Add(id)
}

// RemoveRootBlock removes a solid entry points from the map.
func (s *State) RemoveRootBlock(id iotago.BlockID) {
	s.evictionMutex.RLock()
	defer s.evictionMutex.RUnlock()

	if rootBlocks := s.rootBlocks.Get(id.Index()); rootBlocks != nil && rootBlocks.Delete(id) {
		if err := s.storage.RootBlocks.Delete(id); err != nil {
			panic(err)
		}
	}
}

// IsRootBlock returns true if the given block is a root block.
func (s *State) IsRootBlock(id iotago.BlockID) (has bool) {
	// TODO: shouldn't this be just included in the snapshot and not hardcoded here?
	// if id == iotago.EmptyBlockID() {
	// 	return true
	// }

	s.evictionMutex.RLock()
	defer s.evictionMutex.RUnlock()

	if id.Index() <= s.delayedBlockEvictionThreshold(s.lastEvictedSlot) || id.Index() > s.lastEvictedSlot {
		return false
	}

	slotBlocks := s.rootBlocks.Get(id.Index(), false)

	return slotBlocks != nil && slotBlocks.Has(id)
}

// LatestRootBlocks returns the latest root blocks.
func (s *State) LatestRootBlocks() iotago.BlockIDs {
	rootBlocks := s.latestRootBlocks.Elements()
	if len(rootBlocks) == 0 {
		return iotago.BlockIDs{iotago.EmptyBlockID()}
	}
	return rootBlocks
}

// Export exports the root blocks to the given writer.
func (s *State) Export(writer io.WriteSeeker, evictedSlot iotago.SlotIndex) (err error) {
	return stream.WriteCollection(writer, func() (elementsCount uint64, err error) {
		for currentSlot := s.delayedBlockEvictionThreshold(evictedSlot) + 1; currentSlot <= evictedSlot; currentSlot++ {
			if err = s.storage.RootBlocks.Stream(currentSlot, func(rootBlockID iotago.BlockID, commitmentID iotago.CommitmentID) (err error) {
				if err = stream.WriteSerializable(writer, rootBlockID, iotago.BlockIDLength); err != nil {
					return errors.Wrapf(err, "failed to write root block ID %s", rootBlockID)
				}

				if err = stream.WriteSerializable(writer, commitmentID, iotago.CommitmentIDLength); err != nil {
					return errors.Wrapf(err, "failed to write root block's %s commitment %s", rootBlockID, commitmentID)
				}

				elementsCount++

				return
			}); err != nil {
				return 0, errors.Wrap(err, "failed to stream root blocks")
			}
		}

		return elementsCount, nil
	})
}

// Import imports the root blocks from the given reader.
func (s *State) Import(reader io.ReadSeeker) (err error) {
	var rootBlockID iotago.BlockID
	var commitmentID iotago.CommitmentID

	return stream.ReadCollection(reader, func(i int) error {
		if err = stream.ReadSerializable(reader, &rootBlockID, iotago.BlockIDLength); err != nil {
			return errors.Wrapf(err, "failed to read root block id %d", i)
		}
		if err = stream.ReadSerializable(reader, &commitmentID, iotago.CommitmentIDLength); err != nil {
			return errors.Wrapf(err, "failed to read root block's %s commitment id", rootBlockID)
		}

		s.AddRootBlock(rootBlockID, commitmentID)

		return nil
	})
}

// PopulateFromStorage populates the root blocks from the storage.
func (s *State) PopulateFromStorage(latestCommitmentIndex iotago.SlotIndex) {
	for index := latestCommitmentIndex - s.delayedBlockEvictionThreshold(latestCommitmentIndex); index <= latestCommitmentIndex; index++ {
		_ = s.storage.RootBlocks.Stream(index, func(id iotago.BlockID, commitmentID iotago.CommitmentID) error {
			s.AddRootBlock(id, commitmentID)

			return nil
		})
	}
}

// delayedBlockEvictionThreshold returns the slot index that is the threshold for delayed rootblocks eviction.
func (s *State) delayedBlockEvictionThreshold(index iotago.SlotIndex) (threshold iotago.SlotIndex) {
	return (index - s.optsRootBlocksEvictionDelay - 1).Max(0)
}

// WithRootBlocksEvictionDelay sets the time since confirmation threshold.
func WithRootBlocksEvictionDelay(delay iotago.SlotIndex) options.Option[State] {
	return func(s *State) {
		s.optsRootBlocksEvictionDelay = delay
	}
}
