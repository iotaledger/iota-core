package slotattestation

import (
	"sync"

	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/core/account"
	"github.com/iotaledger/hive.go/core/memstorage"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/attestation"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/notarization/slotnotarization"
	iotago "github.com/iotaledger/iota.go/v4"
)

const (
	DefaultOffsetMinCommittableAge     = 4
	DefaultAttestationCommitmentOffset = slotnotarization.DefaultMinSlotCommittableAge + DefaultOffsetMinCommittableAge
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

		e.HookInitialized(func() {
			cw := e.Storage.Settings().LatestCommitment().Commitment().CumulativeWeight
			index := e.Storage.Settings().LatestCommitment().ID().Index()
			m.Initialize(index, cw)
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

func (m *Manager) Initialize(lastCommitmentIndex iotago.SlotIndex, lastCumulativeWeight uint64) {
	m.commitmentMutex.Lock()
	m.lastCommittedSlot = lastCommitmentIndex
	m.lastCumulativeWeight = lastCumulativeWeight
	m.commitmentMutex.Unlock()

	m.TriggerInitialized()
}

func (m *Manager) Shutdown() {
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

	// Attestations that are older than m.lastCommittedSlot - m.attestationCommitmentOffset don't have any effect, so we ignore them.
	if block.SlotCommitmentID().Index() <= m.computeAttestationCommitmentOffset() {
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

	// Get all attestations for the valid time window of attestationSlotIndex up to index (as we just applied the pending attestations).
	attestationSlotIndex := m.computeAttestationCommitmentOffset()
	attestations := m.tracker.EvictSlot(attestationSlotIndex)

	// Store all attestations of attestationSlotIndex in bucketed storage via ads.Map / sparse merkle tree.
	tree, err := m.attestationStorage(index)
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

	return m.attestationStorage(index)
}

func (m *Manager) computeAttestationCommitmentOffset() iotago.SlotIndex {
	if m.lastCommittedSlot < m.attestationCommitmentOffset {
		return 0
	}

	return m.lastCommittedSlot - m.attestationCommitmentOffset
}

func (m *Manager) attestationStorage(index iotago.SlotIndex) (*ads.Map[iotago.AccountID, iotago.Attestation, *iotago.AccountID, *iotago.Attestation], error) {
	attestationsStorage := m.bucketedStorage(index)
	if attestationsStorage == nil {
		return nil, errors.Errorf("failed to access storage for attestors of slot %d", index)
	}

	return ads.NewMap[iotago.AccountID, iotago.Attestation](attestationsStorage), nil
}
