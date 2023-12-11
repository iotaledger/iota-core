package eviction

import (
	"errors"
	"io"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/hive.go/serializer/v2"
	"github.com/iotaledger/hive.go/serializer/v2/stream"

	// "github.com/iotaledger/hive.go/serializer/v2"
	// "github.com/iotaledger/hive.go/serializer/v2/stream"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/storage/permanent"
	"github.com/iotaledger/iota-core/pkg/storage/prunable/slotstore"
	iotago "github.com/iotaledger/iota.go/v4"
)

const latestNonEmptySlotKey = 1

// State represents the state of the eviction and keeps track of the root blocks.
type State struct {
	Events *Events

	settings             *permanent.Settings
	rootBlockStorageFunc func(iotago.SlotIndex) (*slotstore.Store[iotago.BlockID, iotago.CommitmentID], error)
	lastCommittedSlot    iotago.SlotIndex
	evictionMutex        syncutils.RWMutex
}

// NewState creates a new eviction State.
func NewState(settings *permanent.Settings, rootBlockStorageFunc func(iotago.SlotIndex) (*slotstore.Store[iotago.BlockID, iotago.CommitmentID], error)) (state *State) {
	return &State{
		settings:             settings,
		Events:               NewEvents(),
		rootBlockStorageFunc: rootBlockStorageFunc,
	}
}

func (s *State) Initialize(lastCommittedSlot iotago.SlotIndex) {
	// This marks the slot from which we only have root blocks, so starting with 0 is valid here, since we only have a root block for genesis.
	s.lastCommittedSlot = lastCommittedSlot
}

func (s *State) AdvanceActiveWindowToIndex(slot iotago.SlotIndex) {
	s.evictionMutex.Lock()

	protocolParams := s.settings.APIProvider().APIForSlot(slot).ProtocolParameters()
	genesisSlot := protocolParams.GenesisSlot()
	maxCommittableAge := protocolParams.MaxCommittableAge()

	s.lastCommittedSlot = slot

	if slot < maxCommittableAge+genesisSlot {
		s.evictionMutex.Unlock()
		return
	}

	// We allow a maxCommittableAge window of available root blocks.
	evictionSlot := slot - maxCommittableAge

	// We do not evict slots that are empty
	if evictionSlot >= s.settings.LatestNonEmptySlot() {
		s.evictionMutex.Unlock()
		return
	}

	s.evictionMutex.Unlock()

	s.Events.SlotEvicted.Trigger(evictionSlot)
}

func (s *State) LastEvictedSlot() iotago.SlotIndex {
	s.evictionMutex.RLock()
	defer s.evictionMutex.RUnlock()

	return s.lastCommittedSlot
}

// InRootBlockSlot checks if the Block associated with the given id is too old.
func (s *State) InRootBlockSlot(id iotago.BlockID) bool {
	s.evictionMutex.RLock()
	defer s.evictionMutex.RUnlock()

	return s.withinActiveIndexRange(id.Slot())
}

func (s *State) AllActiveRootBlocks() map[iotago.BlockID]iotago.CommitmentID {
	s.evictionMutex.RLock()
	defer s.evictionMutex.RUnlock()

	activeRootBlocks := make(map[iotago.BlockID]iotago.CommitmentID)
	startSlot, endSlot := s.activeIndexRange(s.lastCommittedSlot)
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

	// We include genesis as a root block if the start of our active window is the genesis slot.
	if startSlot == s.settings.APIProvider().APIForSlot(s.lastCommittedSlot).ProtocolParameters().GenesisSlot() {
		activeRootBlocks[s.settings.APIProvider().CommittedAPI().ProtocolParameters().GenesisBlockID()] = model.NewEmptyCommitment(s.settings.APIProvider().CommittedAPI()).ID()
	}

	return activeRootBlocks
}

func (s *State) LatestActiveRootBlock() (iotago.BlockID, iotago.CommitmentID) {
	s.evictionMutex.RLock()
	defer s.evictionMutex.RUnlock()

	startSlot, endSlot := s.activeIndexRange(s.lastCommittedSlot)
	for slot := endSlot; slot >= startSlot; slot-- {
		// We assume the cache is always populated for the latest slots.
		storage, err := s.rootBlockStorageFunc(slot)
		// Slot too old, it was pruned.
		if err != nil {
			continue
		}

		var latestRootBlock iotago.BlockID
		var latestSlotCommitmentID iotago.CommitmentID

		_ = storage.Stream(func(id iotago.BlockID, commitmentID iotago.CommitmentID) error {
			latestRootBlock = id
			latestSlotCommitmentID = commitmentID

			// We want the newest rootblock.
			return errors.New("stop iteration")
		})

		// We found the most recent root block in this slot.
		if latestRootBlock != iotago.EmptyBlockID {
			return latestRootBlock, latestSlotCommitmentID
		}
	}

	// Fallback to genesis block and genesis commitment if we have no active root blocks.
	return s.settings.APIProvider().CommittedAPI().ProtocolParameters().GenesisBlockID(), model.NewEmptyCommitment(s.settings.APIProvider().CommittedAPI()).ID()
}

// AddRootBlock inserts a solid entry point to the seps map.
func (s *State) AddRootBlock(id iotago.BlockID, commitmentID iotago.CommitmentID) {
	s.evictionMutex.RLock()
	defer s.evictionMutex.RUnlock()

	// The rootblock is too old, ignore it.
	if id.Slot() < lo.Return1(s.activeIndexRange(s.lastCommittedSlot)) {
		return
	}

	if err := lo.PanicOnErr(s.rootBlockStorageFunc(id.Slot())).Store(id, commitmentID); err != nil {
		panic(ierrors.Wrapf(err, "failed to store root block %s", id))
	}

	s.settings.AdvanceLatestNonEmptySlot(id.Slot())
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

	start, _ := s.activeIndexRange(lowerTarget)
	latestNonEmptySlot := s.settings.APIProvider().APIForSlot(targetSlot).ProtocolParameters().GenesisSlot()

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

	s.settings.SetLatestNonEmptySlot(latestNonEmptySlot)

	return nil
}

func (s *State) Rollback(lowerTarget iotago.SlotIndex, targetSlot iotago.SlotIndex) error {
	s.evictionMutex.RLock()
	defer s.evictionMutex.RUnlock()

	start, _ := s.activeIndexRange(lowerTarget)
	latestNonEmptySlot := s.settings.APIProvider().APIForSlot(targetSlot).ProtocolParameters().GenesisSlot()

	for currentSlot := start; currentSlot <= targetSlot; currentSlot++ {
		_, err := s.rootBlockStorageFunc(currentSlot)
		if err != nil {
			continue
		}

		latestNonEmptySlot = currentSlot
	}

	s.settings.SetLatestNonEmptySlot(latestNonEmptySlot)

	return nil
}

func (s *State) Reset() { /* nothing to reset but comply with interface */ }

func (s *State) activeIndexRange(targetSlot iotago.SlotIndex) (startSlot iotago.SlotIndex, endSlot iotago.SlotIndex) {
	protocolParams := s.settings.APIProvider().APIForSlot(targetSlot).ProtocolParameters()
	genesisSlot := protocolParams.GenesisSlot()
	maxCommittableAge := protocolParams.MaxCommittableAge()

	if targetSlot < maxCommittableAge+genesisSlot {
		return genesisSlot, targetSlot
	}

	rootBlocksWindowStart := targetSlot - maxCommittableAge + 1

	if latestNonEmptySlot := s.settings.LatestNonEmptySlot(); rootBlocksWindowStart > latestNonEmptySlot {
		rootBlocksWindowStart = latestNonEmptySlot
	}

	return rootBlocksWindowStart, targetSlot
}

func (s *State) withinActiveIndexRange(slot iotago.SlotIndex) bool {
	startSlot, endSlot := s.activeIndexRange(s.lastCommittedSlot)

	return slot >= startSlot && slot <= endSlot
}
