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
	iotago "github.com/iotaledger/iota.go/v4"
)

const (
	PrefixAttestations byte = iota
)

type Manager struct {
	weightsProviderFunc func() *account.Accounts[iotago.AccountID, *iotago.AccountID]

	bucketedStorage func(index iotago.SlotIndex) kvstore.KVStore

	pendingAttestations *memstorage.IndexedStorage[iotago.SlotIndex, iotago.AccountID, *iotago.Attestation]
	tracker             *Tracker

	lastCommittedSlot           iotago.SlotIndex
	lastCumulativeWeight        uint64
	attestationCommitmentOffset iotago.SlotIndex
	commitmentMutex             sync.RWMutex

	module.Module
}

func NewProvider() module.Provider[*engine.Engine, attestation.Attestation] {
	return module.Provide(func(e *engine.Engine) attestation.Attestation {
		m := NewManager(2, e.Storage.Prunable.Attestations, e.SybilProtection.Accounts)

		e.HookInitialized(func() {
			cw := e.Storage.Settings().LatestCommitment().Commitment().CumulativeWeight
			index := e.Storage.Settings().LatestCommitment().ID().Index()
			m.Initialize(index, cw)
		})

		return m
	})
}

func NewManager(attestationCommitmentOffset iotago.SlotIndex, bucketedStorage func(index iotago.SlotIndex) kvstore.KVStore, weightsProviderFunc func() *account.Accounts[iotago.AccountID, *iotago.AccountID]) *Manager {
	m := &Manager{
		attestationCommitmentOffset: attestationCommitmentOffset,
		weightsProviderFunc:         weightsProviderFunc,
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
	m.commitmentMutex.RLock()
	defer m.commitmentMutex.RUnlock()

	// TODO: check if blockIDIndex is above lastCommittedSlotIndex, otherwise ignore.
	//  check if commitment index > blockID index - drift, otherwise ignore.
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
	attestations := m.tracker.Attestations(attestationSlotIndex)
	m.tracker.EvictSlot(attestationSlotIndex)

	// Store all attestations of attestationSlotIndex in bucketed storage.
	attestationsStorage, err := m.bucketedStorage(attestationSlotIndex).WithExtendedRealm(kvstore.Realm{PrefixAttestations})
	if err != nil {
		return 0, nil, errors.Wrapf(err, "failed to access storage for attestors of slot %d", index)
	}
	tree := ads.NewMap[iotago.AccountID, iotago.Attestation](attestationsStorage)

	// Add all attestations to the tree and calculate the new cumulative weight.
	for _, a := range attestations {
		// TODO: which weight are we using here? The current one? Or the one of the slot of the attestation/commitmentID?
		if attestorWeight, exists := m.weightsProviderFunc().Get(a.IssuerID); exists {
			tree.Set(a.IssuerID, a)

			m.lastCumulativeWeight += uint64(attestorWeight)
		}
	}

	// TODO: set this at the end or beginning?
	m.lastCommittedSlot = index

	return m.lastCumulativeWeight, tree, nil
}

func (m *Manager) computeAttestationCommitmentOffset() iotago.SlotIndex {
	if m.lastCommittedSlot < m.attestationCommitmentOffset {
		return 0
	}

	return m.lastCommittedSlot - m.attestationCommitmentOffset
}
