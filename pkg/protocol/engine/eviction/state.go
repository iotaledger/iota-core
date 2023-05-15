package eviction

import (
	"fmt"
	"io"
	"math"
	"sync"

	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/core/memstorage"
	"github.com/iotaledger/hive.go/ds/ringbuffer"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/serializer/v2/stream"
	"github.com/iotaledger/iota-core/pkg/storage/prunable"
	iotago "github.com/iotaledger/iota.go/v4"
)

// State represents the state of the eviction and keeps track of the root blocks.
type State struct {
	Events *Events

	rootBlocks           *memstorage.IndexedStorage[iotago.SlotIndex, iotago.BlockID, iotago.CommitmentID]
	latestRootBlocks     *ringbuffer.RingBuffer[iotago.BlockID]
	rootBlockStorageFunc func(iotago.SlotIndex) *prunable.RootBlocks
	lastEvictedSlot      iotago.SlotIndex
	evictionMutex        sync.RWMutex

	optsRootBlocksEvictionDelay iotago.SlotIndex
}

// NewState creates a new eviction State.
func NewState(rootBlockStorageFunc func(iotago.SlotIndex) *prunable.RootBlocks, opts ...options.Option[State]) (state *State) {
	return options.Apply(&State{
		Events:                      NewEvents(),
		rootBlocks:                  memstorage.NewIndexedStorage[iotago.SlotIndex, iotago.BlockID, iotago.CommitmentID](),
		latestRootBlocks:            ringbuffer.NewRingBuffer[iotago.BlockID](8),
		rootBlockStorageFunc:        rootBlockStorageFunc,
		optsRootBlocksEvictionDelay: 3,
	}, opts)
}

func (s *State) Initialize(lastCommittedSlot iotago.SlotIndex) {
	// This marks the slot from which we only have root blocks, so starting with 0 is valid here, since we only have a root block for genesis.
	s.lastEvictedSlot = lastCommittedSlot
}

func (s *State) AdvanceActiveWindowToIndex(index iotago.SlotIndex) {
	s.evictionMutex.Lock()
	s.lastEvictedSlot = index

	if delayedIndex, shouldEvictRootBlocks := s.delayedBlockEvictionThreshold(index); shouldEvictRootBlocks {
		s.rootBlocks.Evict(delayedIndex)
	}

	s.evictionMutex.Unlock()

	s.Events.SlotEvicted.Trigger(index)
}

func (s *State) LastEvictedSlot() iotago.SlotIndex {
	s.evictionMutex.RLock()
	defer s.evictionMutex.RUnlock()

	return s.lastEvictedSlot
}

// EarliestRootCommitmentID returns the earliest commitment that rootblocks are committing to across all rootblocks.
func (s *State) EarliestRootCommitmentID() (earliestCommitment iotago.CommitmentID) {
	s.evictionMutex.RLock()
	defer s.evictionMutex.RUnlock()

	earliestCommitment = iotago.NewSlotIdentifier(math.MaxInt64, [32]byte{})

	start, end := s.activeIndexRange()
	for index := start; index <= end; index++ {
		storage := s.rootBlocks.Get(index)
		if storage == nil {
			continue
		}

		storage.ForEach(func(id iotago.BlockID, commitmentID iotago.CommitmentID) bool {
			// fmt.Println(id, "EarliestRootCommitmentID: ", commitmentID.String(), " ", earliestCommitment.String(), " ", commitmentID.Index(), " ", earliestCommitment.Index())
			if commitmentID.Index() < earliestCommitment.Index() {
				earliestCommitment = commitmentID
			}

			return true
		})
	}

	if earliestCommitment.Index() == math.MaxInt64 {
		return iotago.NewEmptyCommitment().MustID()
	}

	return earliestCommitment
}

// InRootBlockSlot checks if the Block associated with the given id is too old.
func (s *State) InRootBlockSlot(id iotago.BlockID) bool {
	s.evictionMutex.RLock()
	defer s.evictionMutex.RUnlock()

	return s.withinActiveIndexRange(id.Index())
}

func (s *State) ActiveRootBlocks() map[iotago.BlockID]iotago.CommitmentID {
	s.evictionMutex.RLock()
	defer s.evictionMutex.RUnlock()

	activeRootBlocks := make(map[iotago.BlockID]iotago.CommitmentID)
	start, end := s.activeIndexRange()
	fmt.Println("ActiveRootBlocks: ", start, " ", end)
	for index := start; index <= end; index++ {
		storage := s.rootBlocks.Get(index)
		if storage == nil {
			continue
		}

		storage.ForEach(func(id iotago.BlockID, commitmentID iotago.CommitmentID) bool {
			activeRootBlocks[id] = commitmentID

			return true
		})
	}

	return activeRootBlocks
}

// AddRootBlock inserts a solid entry point to the seps map.
func (s *State) AddRootBlock(id iotago.BlockID, commitmentID iotago.CommitmentID) {
	s.evictionMutex.RLock()
	defer s.evictionMutex.RUnlock()

	// The rootblock is too old, ignore it.
	if id.Index() < lo.Return1(s.activeIndexRange()) {
		fmt.Println("ignore AddRootBlock: ", id.String(), id.Index(), lo.Return1(s.activeIndexRange()))
		return
	}
	fmt.Println("AddRootBlock: ", id.String(), id.Index(), lo.Return1(s.activeIndexRange()))

	if s.rootBlocks.Get(id.Index(), true).Set(id, commitmentID) {
		if err := s.rootBlockStorageFunc(id.Index()).Store(id, commitmentID); err != nil {
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
		if err := s.rootBlockStorageFunc(id.Index()).Delete(id); err != nil {
			panic(err)
		}
	}
}

// IsRootBlock returns true if the given block is a root block.
func (s *State) IsRootBlock(id iotago.BlockID) (has bool) {
	s.evictionMutex.RLock()
	defer s.evictionMutex.RUnlock()

	if !s.withinActiveIndexRange(id.Index()) {
		return false
	}

	slotBlocks := s.rootBlocks.Get(id.Index(), false)

	return slotBlocks != nil && slotBlocks.Has(id)
}

// RootBlockCommitmentID returns the commitmentID if it is a known root block.
func (s *State) RootBlockCommitmentID(id iotago.BlockID) (commitmentID iotago.CommitmentID, exists bool) {
	s.evictionMutex.RLock()
	defer s.evictionMutex.RUnlock()

	start, end := s.activeIndexRange()
	fmt.Println("RootBlockCommitmentID: ", id.String(), id.Index(), start, end)
	if !s.withinActiveIndexRange(id.Index()) {
		return iotago.CommitmentID{}, false
	}

	slotBlocks := s.rootBlocks.Get(id.Index(), false)
	if slotBlocks == nil {
		return iotago.CommitmentID{}, false
	}

	return slotBlocks.Get(id)
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
func (s *State) Export(writer io.WriteSeeker, targetSlot iotago.SlotIndex) (err error) {
	s.evictionMutex.RLock()
	defer s.evictionMutex.RUnlock()

	start, _ := s.activeIndexRange()

	return stream.WriteCollection(writer, func() (elementsCount uint64, err error) {
		for currentSlot := start; currentSlot <= targetSlot; currentSlot++ {
			fmt.Println("Export: ", currentSlot)
			if err = s.rootBlockStorageFunc(currentSlot).Stream(func(rootBlockID iotago.BlockID, commitmentID iotago.CommitmentID) (err error) {
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

		fmt.Println("importing root block", rootBlockID, commitmentID)
		s.AddRootBlock(rootBlockID, commitmentID)

		return nil
	})
}

// PopulateFromStorage populates the root blocks from the storage.
func (s *State) PopulateFromStorage(latestCommitmentIndex iotago.SlotIndex) {
	for index := lo.Return1(s.delayedBlockEvictionThreshold(latestCommitmentIndex)); index <= latestCommitmentIndex; index++ {
		_ = s.rootBlockStorageFunc(index).Stream(func(id iotago.BlockID, commitmentID iotago.CommitmentID) error {
			s.AddRootBlock(id, commitmentID)

			return nil
		})
	}
}

func (s *State) activeIndexRange() (start, end iotago.SlotIndex) {
	lastCommitted := s.lastEvictedSlot
	delayed, valid := s.delayedBlockEvictionThreshold(lastCommitted)

	if !valid {
		return 0, lastCommitted
	}

	if delayed+1 > lastCommitted {
		return delayed, lastCommitted
	}

	return delayed + 1, lastCommitted
}

func (s *State) withinActiveIndexRange(index iotago.SlotIndex) bool {
	start, end := s.activeIndexRange()

	return index >= start && index <= end
}

// delayedBlockEvictionThreshold returns the slot index that is the threshold for delayed rootblocks eviction.
func (s *State) delayedBlockEvictionThreshold(slotIndex iotago.SlotIndex) (threshold iotago.SlotIndex, shouldEvict bool) {
	if slotIndex >= s.optsRootBlocksEvictionDelay {
		return slotIndex - s.optsRootBlocksEvictionDelay, true
	}

	return 0, false
}

// WithRootBlocksEvictionDelay sets the time since confirmation threshold.
func WithRootBlocksEvictionDelay(delay iotago.SlotIndex) options.Option[State] {
	return func(s *State) {
		s.optsRootBlocksEvictionDelay = delay
	}
}
