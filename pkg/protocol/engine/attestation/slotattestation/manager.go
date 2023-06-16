package slotattestation

import (
	"sync"

	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/core/account"
	"github.com/iotaledger/hive.go/core/memstorage"
	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/runtime/module"
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
//
//  1. It stores "future" attestations temporarily until the corresponding slot becomes committable.
//
//  2. When a slot is being committed:
//     a. Apply the future attestations of the committed slot to the pending attestations window.
//     b. Determine attestations from window for current slot = committed slot - attestationCommitmentOffset and store in bucketed storage as committed attestations.
//     c. Compute the new cumulative weight based on the newly committed attestations and the previous weight.
//
// Example with attestationCommitmentOffset = 2 and lastCommittedSlot = 12:
//
//	|      9     |     10     |     11     |     12     |     13     |     14     |     15     |
//	|            |            |            |            |            |            |            |
//	|            |            |<-----------|----------->|<-----------|------------|------------>
//	|            |            | Pending Attestations    | Future Attestations     |            |
//	|            |            |            |            |            |            |            |
//	|<-----------|------------|------------|----------->|            |            |            |
//	| Committed Attestations                            |            |            |            |
//
//	Explanation:
//	- Slots before 12 are committed according to their window.
//	- When committing slot 13, the future attestations from 13 are applied to the pending attestations window and the attestations to be committed determined.
//		- Attestations to be committed at 13 are those at the lower bound of the window, issuer.e. slot 11
//		- The lower bound of the window is advanced to slot 12.
//		- The last committed slot is advanced to 13.
//	- future attestations: everything above lastCommittedSlot
//	- pending attestations: sliding window of size attestationCommitmentOffset, add to pending attestations (ratified accepted) blocks of slot that we are committing
//		- obtain and evict from it attestations that *commit to* lastCommittedSlot-attestationCommitmentOffset
//	- committed attestations: retrieved at slot that we are committing, stored at slot lastCommittedSlot-attestationCommitmentOffset
type Manager struct {
	committeeFunc               func() *account.Accounts[iotago.AccountID, *iotago.AccountID]
	attestationCommitmentOffset iotago.SlotIndex

	futureAttestations  *memstorage.IndexedStorage[iotago.SlotIndex, iotago.AccountID, *iotago.Attestation]
	pendingAttestations *memstorage.IndexedStorage[iotago.SlotIndex, iotago.AccountID, *iotago.Attestation]
	bucketedStorage     func(index iotago.SlotIndex) kvstore.KVStore // contains committed attestations

	lastCommittedSlot    iotago.SlotIndex
	lastCumulativeWeight uint64

	commitmentMutex sync.RWMutex

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
		futureAttestations:          memstorage.NewIndexedStorage[iotago.SlotIndex, iotago.AccountID, *iotago.Attestation](),
		pendingAttestations:         memstorage.NewIndexedStorage[iotago.SlotIndex, iotago.AccountID, *iotago.Attestation](),
	}
	m.TriggerConstructed()

	return m
}

func (m *Manager) Shutdown() {
	if err := m.writeToDisk(); err != nil {
		panic(err)
	}
	m.TriggerStopped()
}

func (m *Manager) AttestationCommitmentOffset() iotago.SlotIndex {
	return m.attestationCommitmentOffset
}

// Get returns the attestations that are included in the commitment of the given slot as list.
// If attestationCommitmentOffset=3 and commitment is 10, then the returned attestations are blocks from 7 to 10 that commit to at least 7.
func (m *Manager) Get(index iotago.SlotIndex) (attestations []*iotago.Attestation, err error) {
	adsMap, err := m.GetMap(index)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get attestations for slot %d", index)
	}

	attestations = make([]*iotago.Attestation, 0)
	if err := adsMap.Stream(func(_ iotago.AccountID, attestation *iotago.Attestation) bool {
		attestations = append(attestations, attestation)

		return true
	}); err != nil {
		return nil, errors.Wrapf(err, "failed to stream attestations for slot %d", index)
	}

	return attestations, nil
}

// GetMap returns the attestations that are included in the commitment of the given slot as ads.Map.
// If attestationCommitmentOffset=3 and commitment is 10, then the returned attestations are blocks from 7 to 10 that commit to at least 7.
func (m *Manager) GetMap(index iotago.SlotIndex) (*ads.Map[iotago.AccountID, iotago.Attestation, *iotago.AccountID, *iotago.Attestation], error) {
	m.commitmentMutex.RLock()
	defer m.commitmentMutex.RUnlock()

	if index > m.lastCommittedSlot {
		return nil, errors.Errorf("slot %d is newer than last committed slot %d", index, m.lastCommittedSlot)
	}
	cutoffIndex, isValid := m.computeAttestationCommitmentOffset(index)
	if !isValid {
		return nil, errors.Errorf("slot %d is smaller than attestation cutoffIndex %d thus we don't have attestations", index, cutoffIndex)
	}

	return m.adsMapStorage(cutoffIndex)
}

// AddAttestationFromBlock adds an attestation from a block to the future attestations (beyond the attestation window).
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

	newAttestation := iotago.NewAttestation(block.Block())

	// We keep only the latest attestation for each committee member.
	m.futureAttestations.Get(block.ID().Index(), true).Compute(block.Block().IssuerID, func(currentValue *iotago.Attestation, exists bool) *iotago.Attestation {
		if !exists {
			return newAttestation
		}

		// Replace the attestation only if the new one is greater.
		if newAttestation.Compare(currentValue) == 1 {
			return newAttestation
		}

		return currentValue
	})
}

func (m *Manager) applyToPendingAttestations(attestation *iotago.Attestation, cutoffIndex iotago.SlotIndex) {
	if attestation.SlotCommitmentID.Index() < cutoffIndex {
		return
	}

	updated := true

	updatedAttestation := m.pendingAttestations.Get(cutoffIndex, true).Compute(attestation.IssuerID, func(currentValue *iotago.Attestation, exists bool) *iotago.Attestation {
		if !exists {
			return attestation
		}

		// Replace the stored attestation only if the new one is greater.
		if attestation.Compare(currentValue) == 1 {
			return attestation
		}

		updated = false

		return currentValue
	})

	// The new attestation is smaller or equal the previousAttestation. There's no need to update pendingAttestations.
	if !updated {
		return
	}

	for i := cutoffIndex; i <= updatedAttestation.SlotCommitmentID.Index(); i++ {
		m.pendingAttestations.Get(i, true).Set(attestation.IssuerID, updatedAttestation)
	}
}

func (m *Manager) determineAttestationsFromWindow(slotIndex iotago.SlotIndex) []*iotago.Attestation {
	slotAttestors := m.pendingAttestations.Get(slotIndex, false)
	if slotAttestors == nil {
		return nil
	}

	return slotAttestors.Values()
}

func (m *Manager) Commit(index iotago.SlotIndex) (newCW uint64, attestationsRoot types.Identifier, err error) {
	m.commitmentMutex.Lock()
	defer m.commitmentMutex.Unlock()

	cutoffIndex, valid := m.computeAttestationCommitmentOffset(index)

	// Remove all future attestations of slot and apply to pending attestations window.
	futureAttestations := m.futureAttestations.Evict(index)
	if futureAttestations != nil {
		futureAttestations.ForEach(func(key iotago.AccountID, value *iotago.Attestation) bool {
			m.applyToPendingAttestations(value, cutoffIndex)

			return true
		})
	}

	if !valid {
		m.lastCommittedSlot = index
		return 0, types.Identifier{}, nil
	}

	// Get all attestations for the valid time window of cutoffIndex up to index (as we just applied the pending attestations).
	attestations := m.determineAttestationsFromWindow(cutoffIndex)
	m.pendingAttestations.Evict(cutoffIndex)

	// Store all attestations of cutoffIndex in bucketed storage via ads.Map / sparse merkle tree -> committed attestations.
	tree, err := m.adsMapStorage(cutoffIndex)
	if err != nil {
		return 0, types.Identifier{}, errors.Wrapf(err, "failed to get attestation storage when committing slot %d", index)
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

	return m.lastCumulativeWeight, tree.Root(), nil
}

func (m *Manager) computeAttestationCommitmentOffset(slot iotago.SlotIndex) (cutoffIndex iotago.SlotIndex, isValid bool) {
	if slot < m.attestationCommitmentOffset {
		return 0, false
	}

	return slot - m.attestationCommitmentOffset, true
}
