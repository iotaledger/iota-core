package eviction

import (
	"io"

	"github.com/iotaledger/hive.go/core/memstorage"
	"github.com/iotaledger/hive.go/ds/ringbuffer"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/hive.go/serializer/v2/stream"
	"github.com/iotaledger/iota-core/pkg/storage/prunable/slotstore"
	iotago "github.com/iotaledger/iota.go/v4"
)

const latestNonEmptySlotKey = 1

// State represents the state of the eviction and keeps track of the root blocks.
type State struct {
	Events *Events

	rootBlocks           *memstorage.IndexedStorage[iotago.SlotIndex, iotago.BlockID, iotago.CommitmentID]
	latestRootBlocks     *ringbuffer.RingBuffer[iotago.BlockID]
	rootBlockStorageFunc func(iotago.SlotIndex) (*slotstore.Store[iotago.BlockID, iotago.CommitmentID], error)
	lastEvictedSlot      iotago.SlotIndex
	latestNonEmptyStore  kvstore.KVStore
	evictionMutex        syncutils.RWMutex

	optsRootBlocksEvictionDelay iotago.SlotIndex
}

// NewState creates a new eviction State.
func NewState(latestNonEmptyStore kvstore.KVStore, rootBlockStorageFunc func(iotago.SlotIndex) (*slotstore.Store[iotago.BlockID, iotago.CommitmentID], error), opts ...options.Option[State]) (state *State) {
	return options.Apply(&State{
		Events:                      NewEvents(),
		rootBlocks:                  memstorage.NewIndexedStorage[iotago.SlotIndex, iotago.BlockID, iotago.CommitmentID](),
		latestRootBlocks:            ringbuffer.NewRingBuffer[iotago.BlockID](8),
		rootBlockStorageFunc:        rootBlockStorageFunc,
		latestNonEmptyStore:         latestNonEmptyStore,
		optsRootBlocksEvictionDelay: 3,
	}, opts)
}

func (s *State) Initialize(lastCommittedSlot iotago.SlotIndex) {
	// This marks the slot from which we only have root blocks, so starting with 0 is valid here, since we only have a root block for genesis.
	s.lastEvictedSlot = lastCommittedSlot
}

func (s *State) AdvanceActiveWindowToIndex(slot iotago.SlotIndex) {
	s.evictionMutex.Lock()
	s.lastEvictedSlot = slot

	if delayedIndex, shouldEvictRootBlocks := s.delayedBlockEvictionThreshold(slot); shouldEvictRootBlocks {
		// Remember the last slot outside our cache window that has root blocks.
		if evictedSlot := s.rootBlocks.Evict(delayedIndex); evictedSlot != nil && evictedSlot.Size() > 0 {
			s.setLatestNonEmptySlot(delayedIndex)
		}
	}

	s.evictionMutex.Unlock()

	s.Events.SlotEvicted.Trigger(slot)
}

func (s *State) LastEvictedSlot() iotago.SlotIndex {
	s.evictionMutex.RLock()
	defer s.evictionMutex.RUnlock()

	return s.lastEvictedSlot
}

// InRootBlockSlot checks if the Block associated with the given id is too old.
func (s *State) InRootBlockSlot(id iotago.BlockID) bool {
	s.evictionMutex.RLock()
	defer s.evictionMutex.RUnlock()

	return s.withinActiveIndexRange(id.Slot())
}

func (s *State) ActiveRootBlocks() map[iotago.BlockID]iotago.CommitmentID {
	s.evictionMutex.RLock()
	defer s.evictionMutex.RUnlock()

	activeRootBlocks := make(map[iotago.BlockID]iotago.CommitmentID)
	startSlot, endSlot := s.activeIndexRange()
	for slot := startSlot; slot <= endSlot; slot++ {
		storage := s.rootBlocks.Get(slot)
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
	if id.Slot() < lo.Return1(s.activeIndexRange()) {
		return
	}

	if s.rootBlocks.Get(id.Slot(), true).Set(id, commitmentID) {
		if err := lo.PanicOnErr(s.rootBlockStorageFunc(id.Slot())).Store(id, commitmentID); err != nil {
			panic(ierrors.Wrapf(err, "failed to store root block %s", id))
		}
	}

	s.latestRootBlocks.Add(id)
}

// RemoveRootBlock removes a solid entry points from the map.
func (s *State) RemoveRootBlock(id iotago.BlockID) {
	s.evictionMutex.RLock()
	defer s.evictionMutex.RUnlock()

	if rootBlocks := s.rootBlocks.Get(id.Slot()); rootBlocks != nil && rootBlocks.Delete(id) {
		if err := lo.PanicOnErr(s.rootBlockStorageFunc(id.Slot())).Delete(id); err != nil {
			panic(err)
		}
	}
}

// IsRootBlock returns true if the given block is a root block.
func (s *State) IsRootBlock(id iotago.BlockID) (has bool) {
	s.evictionMutex.RLock()
	defer s.evictionMutex.RUnlock()

	if !s.withinActiveIndexRange(id.Slot()) {
		return false
	}

	slotBlocks := s.rootBlocks.Get(id.Slot(), false)

	return slotBlocks != nil && slotBlocks.Has(id)
}

// RootBlockCommitmentID returns the commitmentID if it is a known root block.
func (s *State) RootBlockCommitmentID(id iotago.BlockID) (commitmentID iotago.CommitmentID, exists bool) {
	s.evictionMutex.RLock()
	defer s.evictionMutex.RUnlock()

	if !s.withinActiveIndexRange(id.Slot()) {
		return iotago.CommitmentID{}, false
	}

	slotBlocks := s.rootBlocks.Get(id.Slot(), false)
	if slotBlocks == nil {
		return iotago.CommitmentID{}, false
	}

	return slotBlocks.Get(id)
}

// LatestRootBlocks returns the latest root blocks.
func (s *State) LatestRootBlocks() iotago.BlockIDs {
	rootBlocks := s.latestRootBlocks.ToSlice()
	if len(rootBlocks) == 0 {
		return iotago.BlockIDs{iotago.EmptyBlockID}
	}

	return rootBlocks
}

// Export exports the root blocks to the given writer.
// The lowerTarget is usually going to be the last finalized slot because Rootblocks are special when creating a snapshot.
// They not only are needed as a Tangle root on the slot we're targeting to export (usually last committed slot) but also to derive the rootcommitment.
// The rootcommitment, however, must not depend on the committed slot but on the finalized slot. Otherwise, we could never switch a chain after committing (as the rootcommitment is our genesis and we don't solidify/switch chains below it).
func (s *State) Export(writer io.WriteSeeker, lowerTarget iotago.SlotIndex, targetSlot iotago.SlotIndex) (err error) {
	s.evictionMutex.RLock()
	defer s.evictionMutex.RUnlock()

	start, _ := s.delayedBlockEvictionThreshold(lowerTarget)

	latestNonEmptySlot := iotago.SlotIndex(0)

	if err := stream.WriteCollection(writer, func() (elementsCount uint64, err error) {
		for currentSlot := start; currentSlot <= targetSlot; currentSlot++ {
			storage, err := s.rootBlockStorageFunc(currentSlot)
			if err != nil {
				continue
			}
			if err = storage.StreamBytes(func(rootBlockIDBytes []byte, commitmentIDBytes []byte) (err error) {
				if err = stream.WriteBlob(writer, rootBlockIDBytes); err != nil {
					return ierrors.Wrapf(err, "failed to write root block ID %s", rootBlockIDBytes)
				}

				if err = stream.WriteBlob(writer, commitmentIDBytes); err != nil {
					return ierrors.Wrapf(err, "failed to write root block's %s commitment %s", rootBlockIDBytes, commitmentIDBytes)
				}

				elementsCount++

				return
			}); err != nil {
				return 0, ierrors.Wrap(err, "failed to stream root blocks")
			}

			latestNonEmptySlot = currentSlot
		}

		return elementsCount, nil
	}); err != nil {
		return ierrors.Wrap(err, "failed to write root blocks")
	}

	if latestNonEmptySlot > s.optsRootBlocksEvictionDelay {
		latestNonEmptySlot -= s.optsRootBlocksEvictionDelay
	} else {
		latestNonEmptySlot = 0
	}

	if err := stream.WriteSerializable(writer, latestNonEmptySlot, iotago.SlotIndexLength); err != nil {
		return ierrors.Wrap(err, "failed to write latest non empty slot")
	}

	return nil
}

// Import imports the root blocks from the given reader.
func (s *State) Import(reader io.ReadSeeker) error {
	if err := stream.ReadCollection(reader, func(i int) error {

		blockIDBytes, err := stream.ReadBlob(reader)
		if err != nil {
			return ierrors.Wrapf(err, "failed to read root block id %d", i)
		}

		rootBlockID, _, err := iotago.BlockIDFromBytes(blockIDBytes)
		if err != nil {
			return ierrors.Wrapf(err, "failed to parse root block id %d", i)
		}

		commitmentIDBytes, err := stream.ReadBlob(reader)
		if err != nil {
			return ierrors.Wrapf(err, "failed to read root block's %s commitment id", rootBlockID)
		}

		commitmentID, _, err := iotago.CommitmentIDFromBytes(commitmentIDBytes)
		if err != nil {
			return ierrors.Wrapf(err, "failed to parse root block's %s commitment id", rootBlockID)
		}

		if s.rootBlocks.Get(rootBlockID.Slot(), true).Set(rootBlockID, commitmentID) {
			if err := lo.PanicOnErr(s.rootBlockStorageFunc(rootBlockID.Slot())).Store(rootBlockID, commitmentID); err != nil {
				panic(ierrors.Wrapf(err, "failed to store root block %s", rootBlockID))
			}
		}

		return nil
	}); err != nil {
		return ierrors.Wrap(err, "failed to read root blocks")
	}

	latestNonEmptySlotBytes, err := stream.ReadBytes(reader, iotago.SlotIndexLength)
	if err != nil {
		return ierrors.Wrap(err, "failed to read latest non empty slot")
	}

	latestNonEmptySlot, _, err := iotago.SlotIndexFromBytes(latestNonEmptySlotBytes)
	if err != nil {
		return ierrors.Wrap(err, "failed to parse latest non empty slot")
	}

	s.setLatestNonEmptySlot(latestNonEmptySlot)

	return nil
}

func (s *State) Rollback(lowerTarget, targetIndex iotago.SlotIndex) error {
	s.evictionMutex.RLock()
	defer s.evictionMutex.RUnlock()

	start, _ := s.delayedBlockEvictionThreshold(lowerTarget)
	latestNonEmptySlot := iotago.SlotIndex(0)

	for currentSlot := start; currentSlot <= targetIndex; currentSlot++ {
		_, err := s.rootBlockStorageFunc(currentSlot)
		if err != nil {
			continue
		}

		latestNonEmptySlot = currentSlot
	}

	if latestNonEmptySlot > s.optsRootBlocksEvictionDelay {
		latestNonEmptySlot -= s.optsRootBlocksEvictionDelay
	} else {
		latestNonEmptySlot = 0
	}

	if err := s.latestNonEmptyStore.Set([]byte{latestNonEmptySlotKey}, latestNonEmptySlot.MustBytes()); err != nil {
		return ierrors.Wrap(err, "failed to store latest non empty slot")
	}

	return nil
}

// PopulateFromStorage populates the root blocks from the storage.
func (s *State) PopulateFromStorage(latestCommitmentSlot iotago.SlotIndex) {
	for slot := lo.Return1(s.delayedBlockEvictionThreshold(latestCommitmentSlot)); slot <= latestCommitmentSlot; slot++ {
		storedRootBlocks, err := s.rootBlockStorageFunc(slot)
		if err != nil {
			continue
		}

		_ = storedRootBlocks.Stream(func(id iotago.BlockID, commitmentID iotago.CommitmentID) error {
			s.AddRootBlock(id, commitmentID)

			return nil
		})
	}
}

// latestNonEmptySlot returns the latest slot that contains a rootblock.
func (s *State) latestNonEmptySlot() iotago.SlotIndex {
	latestNonEmptySlotBytes, err := s.latestNonEmptyStore.Get([]byte{latestNonEmptySlotKey})
	if err != nil {
		if ierrors.Is(err, kvstore.ErrKeyNotFound) {
			return 0
		}
		panic(ierrors.Wrap(err, "failed to get latest non empty slot"))
	}

	latestNonEmptySlot, _, err := iotago.SlotIndexFromBytes(latestNonEmptySlotBytes)
	if err != nil {
		panic(ierrors.Wrap(err, "failed to parse latest non empty slot"))
	}

	return latestNonEmptySlot
}

// setLatestNonEmptySlot sets the latest slot that contains a rootblock.
func (s *State) setLatestNonEmptySlot(slot iotago.SlotIndex) {
	if err := s.latestNonEmptyStore.Set([]byte{latestNonEmptySlotKey}, slot.MustBytes()); err != nil {
		panic(ierrors.Wrap(err, "failed to store latest non empty slot"))
	}
}

func (s *State) activeIndexRange() (startSlot, endSlot iotago.SlotIndex) {
	lastCommittedSlot := s.lastEvictedSlot
	delayedSlot, valid := s.delayedBlockEvictionThreshold(lastCommittedSlot)

	if !valid {
		return 0, lastCommittedSlot
	}

	if delayedSlot+1 > lastCommittedSlot {
		return delayedSlot, lastCommittedSlot
	}

	return delayedSlot + 1, lastCommittedSlot
}

func (s *State) withinActiveIndexRange(slot iotago.SlotIndex) bool {
	startSlot, endSlot := s.activeIndexRange()

	return slot >= startSlot && slot <= endSlot
}

// delayedBlockEvictionThreshold returns the slot index that is the threshold for delayed rootblocks eviction.
func (s *State) delayedBlockEvictionThreshold(slot iotago.SlotIndex) (thresholdSlot iotago.SlotIndex, shouldEvict bool) {
	if slot < s.optsRootBlocksEvictionDelay {
		return 0, false
	}

	// Check if we have root blocks up to the eviction point.
	for ; slot >= s.lastEvictedSlot; slot-- {
		if rb := s.rootBlocks.Get(slot); rb != nil {
			if rb.Size() > 0 {
				if slot >= s.optsRootBlocksEvictionDelay {
					return slot - s.optsRootBlocksEvictionDelay, true
				}

				return 0, false
			}
		}
	}

	// If we didn't find any root blocks, we have to fallback to the latestNonEmptySlot before the eviction point.
	if latestNonEmptySlot := s.latestNonEmptySlot(); latestNonEmptySlot >= s.optsRootBlocksEvictionDelay {
		return latestNonEmptySlot - s.optsRootBlocksEvictionDelay, true
	}

	return 0, false
}

// WithRootBlocksEvictionDelay sets the time since confirmation threshold.
func WithRootBlocksEvictionDelay(evictionDelay iotago.SlotIndex) options.Option[State] {
	return func(s *State) {
		s.optsRootBlocksEvictionDelay = evictionDelay
	}
}
