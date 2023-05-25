package slotattestation

import (
	"fmt"
	"io"
	"sync"

	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/core/account"
	"github.com/iotaledger/hive.go/core/memstorage"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/serializer/v2/stream"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/attestation"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	iotago "github.com/iotaledger/iota.go/v4"
)

const (
	DefaultAttestationCommitmentOffset = 4

	prefixAttestationsADSMap byte = iota
	prefixAttestationsTracker
)

// Manager is the manager of slot attestations. It works in two phases:
//  1. It stores "pending" attestations temporarily until the corresponding slot becomes committable.
//  2. When a slot is committed:
//     a. Apply the pending attestations of the committed slot.
//     b. It computes the cumulative weight of the committed slot.
//     c. Get all attestations for attestation slot = committed slot - attestationCommitmentOffset and store in bucketed storage.
//     d. Compute the cumulative weight of the attestation slot.
type Manager struct {
	committeeFunc func() *account.Accounts[iotago.AccountID, *iotago.AccountID]

	bucketedStorage func(index iotago.SlotIndex) kvstore.KVStore

	pendingAttestations *memstorage.IndexedStorage[iotago.SlotIndex, iotago.AccountID, *iotago.Attestation]
	tracker             *Tracker

	lastCommittedSlot           iotago.SlotIndex
	lastCumulativeWeight        uint64
	attestationCommitmentOffset iotago.SlotIndex
	commitmentMutex             sync.RWMutex

	module.Module
}

func NewProvider(attestationCommitmentOffset iotago.SlotIndex) module.Provider[*engine.Engine, attestation.Attestations] {
	return module.Provide(func(e *engine.Engine) attestation.Attestations {
		m := NewManager(attestationCommitmentOffset, e.Storage.Prunable.Attestations, e.SybilProtection.Accounts)

		e.Storage.Settings().HookInitialized(func() {
			m.commitmentMutex.Lock()
			m.lastCommittedSlot = e.Storage.Settings().LatestCommitment().ID().Index()
			m.lastCumulativeWeight = e.Storage.Settings().LatestCommitment().Commitment().CumulativeWeight
			m.commitmentMutex.Unlock()
		})

		return m
	})
}

func NewManager(attestationCommitmentOffset iotago.SlotIndex, bucketedStorage func(index iotago.SlotIndex) kvstore.KVStore, committeeFunc func() *account.Accounts[iotago.AccountID, *iotago.AccountID]) *Manager {
	m := &Manager{
		attestationCommitmentOffset: attestationCommitmentOffset,
		committeeFunc:               committeeFunc,
		bucketedStorage:             bucketedStorage,
		pendingAttestations:         memstorage.NewIndexedStorage[iotago.SlotIndex, iotago.AccountID, *iotago.Attestation](),
	}
	m.tracker = NewTracker(m.computeAttestationCommitmentOffset)
	m.TriggerConstructed()

	return m
}

func (m *Manager) Shutdown() {
	if err := m.writeToDisk(); err != nil {
		panic(err)
	}
	m.TriggerStopped()
}

func (m *Manager) AddAttestationFromBlock(block *blocks.Block) {
	// Only track attestations of active committee members.
	if _, exists := m.committeeFunc().Get(block.Block().IssuerID); !exists {
		return
	}

	m.commitmentMutex.RLock()
	defer m.commitmentMutex.RUnlock()

	// We only care about attestations that are newer than the last committed slot.
	if block.ID().Index() <= m.lastCommittedSlot {
		return
	}

	cutoffIndex, isValid := m.computeAttestationCommitmentOffset()
	if !isValid {
		return
	}

	// Attestations that are older than m.lastCommittedSlot - m.attestationCommitmentOffset don't have any effect, so we ignore them.
	if block.SlotCommitmentID().Index() <= cutoffIndex {
		return
	}

	newAttestation := iotago.NewAttestation(block.Block())

	storage := m.pendingAttestations.Get(block.ID().Index(), true)
	storage.Compute(block.Block().IssuerID, func(currentValue *iotago.Attestation, exists bool) *iotago.Attestation {
		if !exists {
			return newAttestation
		}

		// Replace the stored attestation only if the new one is greater.
		if newAttestation.Compare(currentValue) == 1 {
			return newAttestation
		}

		return currentValue
	})
}

func (m *Manager) Commit(index iotago.SlotIndex) (newCW uint64, attestationsMap *ads.Map[iotago.AccountID, iotago.Attestation, *iotago.AccountID, *iotago.Attestation], err error) {
	m.commitmentMutex.Lock()
	defer m.commitmentMutex.Unlock()

	// Remove all pending attestations of slot and apply to tracker.
	pendingAttestations := m.pendingAttestations.Evict(index)
	if pendingAttestations != nil {
		pendingAttestations.ForEach(func(key iotago.AccountID, value *iotago.Attestation) bool {
			m.tracker.TrackAttestation(value)
			return true
		})
	}

	attestationSlotIndex, _ := m.computeAttestationCommitmentOffset()

	// Get all attestations for the valid time window of attestationSlotIndex up to index (as we just applied the pending attestations).
	attestations := m.tracker.EvictSlot(attestationSlotIndex)

	// Store all attestations of attestationSlotIndex in bucketed storage via ads.Map / sparse merkle tree.
	tree, err := m.adsMapStorage(attestationSlotIndex)
	if err != nil {
		return 0, nil, errors.Wrapf(err, "failed to get attestation storage when committing slot %d", index)
	}

	// Add all attestations to the tree and calculate the new cumulative weight.
	for _, a := range attestations {
		// TODO: which weight are we using here? The current one? Or the one of the slot of the attestation/commitmentID?
		if attestorWeight, exists := m.committeeFunc().Get(a.IssuerID); exists {
			tree.Set(a.IssuerID, a)

			m.lastCumulativeWeight += uint64(attestorWeight)
		}
	}

	m.lastCommittedSlot = index

	return m.lastCumulativeWeight, tree, nil
}

func (m *Manager) Get(index iotago.SlotIndex) (*ads.Map[iotago.AccountID, iotago.Attestation, *iotago.AccountID, *iotago.Attestation], error) {
	m.commitmentMutex.RLock()
	defer m.commitmentMutex.RUnlock()

	if index > m.lastCommittedSlot {
		return nil, errors.Errorf("slot %d is newer than last committed slot %d", index, m.lastCommittedSlot)
	}

	return m.adsMapStorage(index)
}

func (m *Manager) computeAttestationCommitmentOffset() (cutoffIndex iotago.SlotIndex, isValid bool) {
	if m.lastCommittedSlot < m.attestationCommitmentOffset {
		return 0, false
	}

	return m.lastCommittedSlot - m.attestationCommitmentOffset, true
}

func (m *Manager) adsMapStorage(index iotago.SlotIndex) (*ads.Map[iotago.AccountID, iotago.Attestation, *iotago.AccountID, *iotago.Attestation], error) {
	attestationsStorage := m.bucketedStorage(index)
	if attestationsStorage == nil {
		return nil, errors.Errorf("failed to access storage for attestors of slot %d", index)
	}
	attestationsStorage, err := attestationsStorage.WithExtendedRealm(kvstore.Realm{prefixAttestationsADSMap})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get extended realm for attestations of slot %d", index)
	}

	return ads.NewMap[iotago.AccountID, iotago.Attestation](attestationsStorage), nil
}

func (m *Manager) trackerStorage(index iotago.SlotIndex) (*kvstore.TypedStore[iotago.AccountID, iotago.Attestation, *iotago.AccountID, *iotago.Attestation], error) {
	trackerStorage := m.bucketedStorage(index)
	if trackerStorage == nil {
		return nil, errors.Errorf("failed to access storage for tracker of slot %d", index)
	}
	trackerStorage, err := trackerStorage.WithExtendedRealm(kvstore.Realm{prefixAttestationsTracker})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get extended realm for tracker of slot %d", index)
	}

	return kvstore.NewTypedStore[iotago.AccountID, iotago.Attestation](trackerStorage), nil
}

func (m *Manager) Import(reader io.ReadSeeker) error {
	m.commitmentMutex.Lock()
	defer m.commitmentMutex.Unlock()

	// Read slot count.
	count, err := stream.Read[iotago.SlotIndex](reader)
	if err != nil {
		return errors.Wrap(err, "failed to read slot")
	}

	for i := 0; i < int(count); i++ {
		// Read slot index.
		slotIndex, err := stream.Read[iotago.SlotIndex](reader)
		if err != nil {
			return errors.Wrap(err, "failed to read slot")
		}

		// Read attestations.
		var attestations []*iotago.Attestation
		if err = stream.ReadCollection(reader, func(i int) error {
			importedAttestation := new(iotago.Attestation)
			if err = stream.ReadSerializable(reader, importedAttestation); err != nil {
				return errors.Wrapf(err, "failed to read attestation %d", i)
			}

			attestations = append(attestations, importedAttestation)

			return nil
		}); err != nil {
			return errors.Wrapf(err, "failed to import attestations for slot %d", slotIndex)
		}

		cutoffIndex, isValid := m.computeAttestationCommitmentOffset()
		if !isValid {
			return nil
		}

		if slotIndex > cutoffIndex {
			for _, a := range attestations {
				m.tracker.TrackAttestation(a)
			}
		} else {
			// We should never be able to import attestations for a slot that is older than the attestation commitment offset.
			panic("commitment not aligned with attestation")
		}
	}

	m.TriggerInitialized()

	return nil
}

func (m *Manager) Export(writer io.WriteSeeker, targetSlot iotago.SlotIndex) error {
	m.commitmentMutex.RLock()
	defer m.commitmentMutex.RUnlock()

	if targetSlot > m.lastCommittedSlot {
		return errors.Errorf("slot %d is newer than last committed slot %d", targetSlot, m.lastCommittedSlot)
	}

	attestationSlotIndex, isValid := m.computeAttestationCommitmentOffset()
	if !isValid {
		if err := stream.Write(writer, uint64(0)); err != nil {
			return errors.Wrap(err, "failed to write slot count")
		}
		return nil
	}

	// Write slot count.
	start := lo.Min(targetSlot-m.attestationCommitmentOffset, 0) + 1
	if err := stream.Write(writer, uint64(start-targetSlot)); err != nil {
		return errors.Wrap(err, "failed to write slot count")
	}

	for i := start; i <= targetSlot; i++ {
		var attestations []*iotago.Attestation
		if i < attestationSlotIndex {
			// Need to get attestations from storage.
			attestationsStorage, err := m.adsMapStorage(i)
			if err != nil {
				return errors.Wrapf(err, "failed to get attestations of slot %d", i)
			}
			err = attestationsStorage.Stream(func(key iotago.AccountID, value *iotago.Attestation) bool {
				attestations = append(attestations, value)
				return true
			})
			if err != nil {
				return errors.Wrapf(err, "failed to stream attestations of slot %d", i)
			}
		} else {
			// Need to get attestations from tracker.
			attestations = m.tracker.Attestations(i)
		}

		// Write slot index.
		if err := stream.Write(writer, uint64(i)); err != nil {
			return errors.Wrapf(err, "failed to write slot %d", i)
		}

		// Write attestations.
		if err := stream.WriteCollection(writer, func() (uint64, error) {
			for _, a := range attestations {
				if writeErr := stream.WriteSerializable(writer, a); writeErr != nil {
					return 0, errors.Wrapf(writeErr, "failed to write attestation %v", a)
				}
			}

			return uint64(len(attestations)), nil
		}); err != nil {
			return errors.Wrapf(err, "failed to write attestations of slot %d", i)
		}
	}

	return nil
}

func (m *Manager) RestoreFromDisk() error {
	m.commitmentMutex.Lock()
	defer m.commitmentMutex.Unlock()

	cutoffIndex, isValid := m.computeAttestationCommitmentOffset()

	for i := cutoffIndex; isValid && i <= m.lastCommittedSlot; i++ {
		storage, err := m.trackerStorage(i)
		if err != nil {
			return errors.Wrapf(err, "failed to get storage for slot %d", i)
		}

		err = storage.Iterate(kvstore.EmptyPrefix, func(key iotago.AccountID, value iotago.Attestation) bool {
			m.tracker.TrackAttestation(&value)
			return true
		})
		if err != nil {
			return errors.Wrapf(err, "failed to iterate over attestations of slot %d", i)
		}
		if err := storage.Clear(); err != nil {
			return errors.Wrapf(err, "failed to clear tracker attestations of slot %d", i)
		}

		// TODO: make sure it's actually deleted
		fmt.Println("RestoreFromDisk: clearing", i)
		err = storage.Iterate(kvstore.EmptyPrefix, func(key iotago.AccountID, value iotago.Attestation) bool {
			fmt.Println("attestation", value)
			return true
		})
		if err != nil {
			panic(err)
		}
	}

	m.TriggerInitialized()

	return nil
}

func (m *Manager) writeToDisk() error {
	m.commitmentMutex.RLock()
	defer m.commitmentMutex.RUnlock()

	cutoffIndex, isValid := m.computeAttestationCommitmentOffset()
	if !isValid {
		return nil
	}

	for i := cutoffIndex; i <= m.lastCommittedSlot; i++ {
		storage, err := m.trackerStorage(i)
		if err != nil {
			return errors.Wrapf(err, "failed to get storage for slot %d", i)
		}

		attestations := m.tracker.Attestations(i)
		for _, a := range attestations {
			if err := storage.Set(a.IssuerID, *a); err != nil {
				return errors.Wrapf(err, "failed to set attestation %v", a)
			}
		}
	}

	return nil
}
