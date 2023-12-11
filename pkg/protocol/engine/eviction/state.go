package eviction

import (
	"io"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/hive.go/serializer/v2"
	"github.com/iotaledger/hive.go/serializer/v2/stream"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/storage/prunable/slotstore"
	iotago "github.com/iotaledger/iota.go/v4"
)

const latestNonEmptySlotKey = 1

// State represents the state of the eviction and keeps track of the root blocks.
type State struct {
	Events *Events

	apiProvider          iotago.APIProvider
	rootBlockStorageFunc func(iotago.SlotIndex) (*slotstore.Store[iotago.BlockID, iotago.CommitmentID], error)
	lastEvictedSlot      iotago.SlotIndex
	latestNonEmptyStore  kvstore.KVStore
	evictionMutex        syncutils.RWMutex
}

// NewState creates a new eviction State.
func NewState(engine *engine.Engine, latestNonEmptyStore kvstore.KVStore, rootBlockStorageFunc func(iotago.SlotIndex) (*slotstore.Store[iotago.BlockID, iotago.CommitmentID], error)) (state *State) {
	return &State{
		apiProvider:          engine,
		Events:               NewEvents(),
		rootBlockStorageFunc: rootBlockStorageFunc,
		latestNonEmptyStore:  latestNonEmptyStore,
	}
}

func (s *State) Initialize(lastCommittedSlot iotago.SlotIndex) {
	// This marks the slot from which we only have root blocks, so starting with 0 is valid here, since we only have a root block for genesis.
	s.lastEvictedSlot = lastCommittedSlot
}

func (s *State) AdvanceActiveWindowToIndex(slot iotago.SlotIndex) {
	s.evictionMutex.Lock()

	protocolParams := s.apiProvider.APIForSlot(slot).ProtocolParameters()
	genesisSlot := protocolParams.GenesisSlot()
	maxCommittableAge := protocolParams.MaxCommittableAge()

	if slot < maxCommittableAge+genesisSlot {
		return
	}

	// We allow a maxCommittableAge window of available root blocks.
	evictionSlot := slot - maxCommittableAge

	// We do not evict slots that are empty
	if evictionSlot >= s.latestNonEmptySlot() {
		return
	}

	s.lastEvictedSlot = slot

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
		// We assume the cache is always populated for the latest slots.
		storage, err := s.rootBlockStorageFunc(slot)
		// Slot too old, it was pruned.
		if err != nil {
			continue
		}

		_ = storage.Stream(func(id iotago.BlockID, commitmentID iotago.CommitmentID) error {
			activeRootBlocks[id] = commitmentID

			return nil
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

	if err := lo.PanicOnErr(s.rootBlockStorageFunc(id.Slot())).Store(id, commitmentID); err != nil {
		panic(ierrors.Wrapf(err, "failed to store root block %s", id))
	}

	s.setLatestNonEmptySlot(id.Slot())
}

// RemoveRootBlock removes a solid entry points from the map.
func (s *State) RemoveRootBlock(id iotago.BlockID) {
	s.evictionMutex.RLock()
	defer s.evictionMutex.RUnlock()

	if err := lo.PanicOnErr(s.rootBlockStorageFunc(id.Slot())).Delete(id); err != nil {
		panic(err)
	}
}

// IsRootBlock returns true if the given block is a root block.
func (s *State) IsRootBlock(id iotago.BlockID) (has bool) {
	s.evictionMutex.RLock()
	defer s.evictionMutex.RUnlock()

	if !s.withinActiveIndexRange(id.Slot()) {
		return false
	}

	storage, err := s.rootBlockStorageFunc(id.Slot())
	if err != nil {
		return false
	}

	return lo.PanicOnErr(storage.Has(id))
}

// RootBlockCommitmentID returns the commitmentID if it is a known root block.
func (s *State) RootBlockCommitmentID(id iotago.BlockID) (commitmentID iotago.CommitmentID, exists bool) {
	s.evictionMutex.RLock()
	defer s.evictionMutex.RUnlock()

	if !s.withinActiveIndexRange(id.Slot()) {
		return iotago.CommitmentID{}, false
	}

	storage, err := s.rootBlockStorageFunc(id.Slot())
	if err != nil {
		return iotago.EmptyCommitmentID, false
	}

	// This return empty value for commitmentID in the case the key is not found.
	commitmentID, exists, err = storage.Load(id)
	if err != nil {
		panic(ierrors.Wrapf(err, "failed to load root block %s", id))
	}

	return commitmentID, exists
}

// Export exports the root blocks to the given writer.
// The lowerTarget is usually going to be the last finalized slot because Rootblocks are special when creating a snapshot.
// They not only are needed as a Tangle root on the slot we're targeting to export (usually last committed slot) but also to derive the rootcommitment.
// The rootcommitment, however, must not depend on the committed slot but on the finalized slot. Otherwise, we could never switch a chain after committing (as the rootcommitment is our genesis and we don't solidify/switch chains below it).
func (s *State) Export(writer io.WriteSeeker, lowerTarget iotago.SlotIndex, targetSlot iotago.SlotIndex) (err error) {
	s.evictionMutex.RLock()
	defer s.evictionMutex.RUnlock()

	start, _ := s.delayedBlockEvictionThreshold(lowerTarget)

	genesisSlot := s.genesisRootBlockFunc().Slot()
	latestNonEmptySlot := genesisSlot

	if err := stream.WriteCollection(writer, serializer.SeriLengthPrefixTypeAsUint32, func() (elementsCount int, err error) {
		for currentSlot := start; currentSlot <= targetSlot; currentSlot++ {
			storage, err := s.rootBlockStorageFunc(currentSlot)
			if err != nil {
				continue
			}
			if err = storage.StreamBytes(func(rootBlockIDBytes []byte, commitmentIDBytes []byte) (err error) {
				if err = stream.WriteBytes(writer, rootBlockIDBytes); err != nil {
					return ierrors.Wrapf(err, "failed to write root block ID %s", rootBlockIDBytes)
				}

				if err = stream.WriteBytes(writer, commitmentIDBytes); err != nil {
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
		latestNonEmptySlot = genesisSlot
	}

	if err := stream.Write(writer, latestNonEmptySlot); err != nil {
		return ierrors.Wrap(err, "failed to write latest non empty slot")
	}

	return nil
}

// Import imports the root blocks from the given reader.
func (s *State) Import(reader io.ReadSeeker) error {
	if err := stream.ReadCollection(reader, serializer.SeriLengthPrefixTypeAsUint32, func(i int) error {
		rootBlockID, err := stream.Read[iotago.BlockID](reader)
		if err != nil {
			return ierrors.Wrapf(err, "failed to read root block id %d", i)
		}

		commitmentID, err := stream.Read[iotago.CommitmentID](reader)
		if err != nil {
			return ierrors.Wrapf(err, "failed to read root block's %s commitment id %d", rootBlockID, i)
		}

		if err := lo.PanicOnErr(s.rootBlockStorageFunc(rootBlockID.Slot())).Store(rootBlockID, commitmentID); err != nil {
			panic(ierrors.Wrapf(err, "failed to store root block %s", rootBlockID))
		}

		return nil
	}); err != nil {
		return ierrors.Wrap(err, "failed to read root blocks")
	}

	latestNonEmptySlot, err := stream.Read[iotago.SlotIndex](reader)
	if err != nil {
		return ierrors.Wrap(err, "failed to read latest non empty slot")
	}

	s.setLatestNonEmptySlot(latestNonEmptySlot)

	return nil
}

func (s *State) Rollback(lowerTarget iotago.SlotIndex, targetIndex iotago.SlotIndex) error {
	s.evictionMutex.RLock()
	defer s.evictionMutex.RUnlock()

	start, _ := s.delayedBlockEvictionThreshold(lowerTarget)
	genesisSlot := s.genesisRootBlockFunc().Slot()
	latestNonEmptySlot := genesisSlot

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
		latestNonEmptySlot = genesisSlot
	}

	if err := s.latestNonEmptyStore.Set([]byte{latestNonEmptySlotKey}, latestNonEmptySlot.MustBytes()); err != nil {
		return ierrors.Wrap(err, "failed to store latest non empty slot")
	}

	return nil
}

func (s *State) Reset() { /* nothing to reset but comply with interface */ }

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

func (s *State) activeIndexRange() (startSlot iotago.SlotIndex, endSlot iotago.SlotIndex) {
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
	genesisSlot := s.genesisRootBlockFunc().Slot()
	if slot < genesisSlot+s.optsRootBlocksEvictionDelay {
		return genesisSlot, false
	}

	var rootBlockInWindow bool
	// Check if we have root blocks up to the eviction point.
	for ; slot >= s.lastEvictedSlot; slot-- {
		storage := lo.PanicOnErr(s.rootBlockStorageFunc(slot))

		_ = storage.StreamKeys(func(_ iotago.BlockID) error {
			if slot >= s.optsRootBlocksEvictionDelay {
				thresholdSlot = slot - s.optsRootBlocksEvictionDelay
				shouldEvict = true
			} else {
				thresholdSlot = genesisSlot
				shouldEvict = false
			}

			rootBlockInWindow = true

			return ierrors.New("stop iteration")
		})
	}

	if rootBlockInWindow {
		return thresholdSlot, shouldEvict
	}

	// If we didn't find any root blocks, we have to fallback to the latestNonEmptySlot before the eviction point.
	if latestNonEmptySlot := s.latestNonEmptySlot(); latestNonEmptySlot >= s.optsRootBlocksEvictionDelay {
		return latestNonEmptySlot - s.optsRootBlocksEvictionDelay, true
	}

	return genesisSlot, false
}
