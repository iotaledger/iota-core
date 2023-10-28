package slotattestation

import (
	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/core/memstorage"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/attestation"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	iotago "github.com/iotaledger/iota.go/v4"
)

const (
	prefixAttestationsADSMap byte = iota
	prefixAttestationsTracker
)

// Manager is the manager of slot attestations. It works in two phases:
//
//  1. It stores "future" attestations temporarily until the corresponding slot becomes committable.
//
//  2. When a slot is being committed:
//     a. Apply the future attestations of the committed slot to the pending attestations window which is of size MaxCommittableAge.
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
//		- Attestations to be committed at 13 are those at the lower bound of the window, i.e. slot 11
//		- The lower bound of the window is advanced to slot 12.
//		- The last committed slot is advanced to 13.
//	- future attestations: everything above lastCommittedSlot
//	- pending attestations: sliding window of size attestationCommitmentOffset, add to pending attestations (ratified accepted) blocks of slot that we are committing
//		- obtain and evict from it attestations that *commit to* lastCommittedSlot-attestationCommitmentOffset
//	- committed attestations: retrieved at slot that we are committing, stored at slot lastCommittedSlot-attestationCommitmentOffset
type Manager struct {
	committeeFunc func(slot iotago.SlotIndex) (*account.SeatedAccounts, bool)

	futureAttestations  *memstorage.IndexedStorage[iotago.SlotIndex, iotago.AccountID, *iotago.Attestation]
	pendingAttestations *memstorage.IndexedStorage[iotago.SlotIndex, iotago.AccountID, *iotago.Attestation]
	bucketedStorage     func(slot iotago.SlotIndex) (kvstore.KVStore, error) // contains committed attestations

	lastCommittedSlot    iotago.SlotIndex
	lastCumulativeWeight uint64

	commitmentMutex syncutils.RWMutex

	apiProvider iotago.APIProvider

	module.Module
}

func NewProvider() module.Provider[*engine.Engine, attestation.Attestations] {
	return module.Provide(func(e *engine.Engine) attestation.Attestations {
		latestCommitment := e.Storage.Settings().LatestCommitment()

		return NewManager(
			latestCommitment.Slot(),
			latestCommitment.CumulativeWeight(),
			e.Storage.Attestations,
			e.SybilProtection.SeatManager().CommitteeInSlot,
			e,
		)
	})
}

func NewManager(
	lastCommittedSlot iotago.SlotIndex,
	lastCumulativeWeight uint64,
	bucketedStorage func(slot iotago.SlotIndex) (kvstore.KVStore, error),
	committeeFunc func(slot iotago.SlotIndex) (*account.SeatedAccounts, bool),
	apiProvider iotago.APIProvider,
) *Manager {
	m := &Manager{
		lastCommittedSlot:    lastCommittedSlot,
		lastCumulativeWeight: lastCumulativeWeight,
		committeeFunc:        committeeFunc,
		bucketedStorage:      bucketedStorage,
		futureAttestations:   memstorage.NewIndexedStorage[iotago.SlotIndex, iotago.AccountID, *iotago.Attestation](),
		pendingAttestations:  memstorage.NewIndexedStorage[iotago.SlotIndex, iotago.AccountID, *iotago.Attestation](),
		apiProvider:          apiProvider,
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

// Get returns the attestations that are included in the commitment of the given slot as list.
// If attestationCommitmentOffset=3 and commitment is 10, then the returned attestations are blocks from 7 to 10 that commit to at least 7.
func (m *Manager) Get(slot iotago.SlotIndex) (attestations []*iotago.Attestation, err error) {
	adsMap, err := m.GetMap(slot)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to get attestations for slot %d", slot)
	}

	attestations = make([]*iotago.Attestation, 0)
	if err := adsMap.Stream(func(_ iotago.AccountID, attestation *iotago.Attestation) error {
		attestations = append(attestations, attestation)

		return nil
	}); err != nil {
		return nil, ierrors.Wrapf(err, "failed to stream attestations for slot %d", slot)
	}

	return attestations, nil
}

// GetMap returns the attestations that are included in the commitment of the given slot as ds.AuthenticatedMap.
// If attestationCommitmentOffset=3 and commitment is 10, then the returned attestations are blocks from 7 to 10 that commit to at least 7.
func (m *Manager) GetMap(slot iotago.SlotIndex) (ads.Map[iotago.Identifier, iotago.AccountID, *iotago.Attestation], error) {
	m.commitmentMutex.RLock()
	defer m.commitmentMutex.RUnlock()

	if slot > m.lastCommittedSlot {
		return nil, ierrors.Errorf("slot %d is newer than last committed slot %d", slot, m.lastCommittedSlot)
	}

	if cutoffIndex, isValid := m.computeAttestationCommitmentOffset(slot); !isValid {
		return nil, ierrors.Errorf("slot %d is smaller than attestation cutoffIndex %d thus we don't have attestations", slot, cutoffIndex)
	}

	return m.attestationsForSlot(slot)
}

// AddAttestationFromValidationBlock adds an attestation from a block to the future attestations (beyond the attestation window).
func (m *Manager) AddAttestationFromValidationBlock(block *blocks.Block) error {
	// Only track validator blocks.
	if _, isValidationBlock := block.ValidationBlock(); !isValidationBlock {
		return nil
	}

	committee, exists := m.committeeFunc(block.ID().Slot())
	if !exists {
		return ierrors.Errorf("committee for slot %d does not exist", block.ID().Slot())
	}
	// Only track attestations of active committee members.
	if _, exists := committee.GetSeat(block.ProtocolBlock().Header.IssuerID); !exists {
		return nil
	}

	m.commitmentMutex.RLock()
	defer m.commitmentMutex.RUnlock()

	// We only care about attestations that are newer than the last committed slot.
	if block.ID().Slot() <= m.lastCommittedSlot {
		return nil
	}

	newAttestation := iotago.NewAttestation(m.apiProvider.APIForSlot(block.ID().Slot()), block.ProtocolBlock())

	// We keep only the latest attestation for each committee member.
	m.futureAttestations.Get(block.ID().Slot(), true).Compute(block.ProtocolBlock().Header.IssuerID, func(currentValue *iotago.Attestation, exists bool) *iotago.Attestation {
		if !exists {
			return newAttestation
		}

		// Replace the attestation only if the new one is greater.
		if newAttestation.Compare(currentValue) == 1 {
			return newAttestation
		}

		return currentValue
	})

	return nil
}

func (m *Manager) applyToPendingAttestations(attestation *iotago.Attestation, cutoffSlot iotago.SlotIndex) {
	if attestation.Header.SlotCommitmentID.Slot() < cutoffSlot {
		return
	}

	updated := true

	updatedAttestation := m.pendingAttestations.Get(cutoffSlot, true).Compute(attestation.Header.IssuerID, func(currentValue *iotago.Attestation, exists bool) *iotago.Attestation {
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

	for i := cutoffSlot; i <= updatedAttestation.Header.SlotCommitmentID.Slot(); i++ {
		m.pendingAttestations.Get(i, true).Set(attestation.Header.IssuerID, updatedAttestation)
	}
}

func (m *Manager) determineAttestationsFromWindow(slot iotago.SlotIndex) []*iotago.Attestation {
	slotAttestors := m.pendingAttestations.Get(slot, false)
	if slotAttestors == nil {
		return nil
	}

	return slotAttestors.Values()
}

func (m *Manager) Commit(slot iotago.SlotIndex) (newCW uint64, attestationsRoot iotago.Identifier, err error) {
	m.commitmentMutex.Lock()
	defer m.commitmentMutex.Unlock()

	cutoffSlot, valid := m.computeAttestationCommitmentOffset(slot)

	// Remove all future attestations of slot and apply to pending attestations window.
	futureAttestations := m.futureAttestations.Evict(slot)
	if futureAttestations != nil {
		futureAttestations.ForEach(func(key iotago.AccountID, value *iotago.Attestation) bool {
			m.applyToPendingAttestations(value, cutoffSlot)

			return true
		})
	}

	if !valid {
		m.lastCommittedSlot = slot
		return 0, iotago.Identifier{}, nil
	}

	// Get all attestations for the valid time window of cutoffSlot up to slot (as we just applied the pending attestations).
	attestations := m.determineAttestationsFromWindow(cutoffSlot)
	m.pendingAttestations.Evict(cutoffSlot)

	// Store all attestations of cutoffSlot in bucketed storage via ads.Map / sparse merkle tree -> committed attestations.
	tree, err := m.attestationsForSlot(slot)
	if err != nil {
		return 0, iotago.Identifier{}, ierrors.Wrapf(err, "failed to get attestation storage when committing slot %d", slot)
	}

	// Add all attestations to the tree and calculate the new cumulative weight.
	committee, exists := m.committeeFunc(slot)
	if !exists {
		return 0, iotago.Identifier{}, ierrors.Wrapf(err, "failed to get committee when committing slot %d", slot)
	}

	for _, a := range attestations {
		// TODO: which weight are we using here? The current one? Or the one of the slot of the attestation/commitmentID?
		if _, exists := committee.GetSeat(a.Header.IssuerID); exists {
			if err := tree.Set(a.Header.IssuerID, a); err != nil {
				return 0, iotago.Identifier{}, ierrors.Wrapf(err, "failed to set attestation %s in tree", a.Header.IssuerID)
			}

			m.lastCumulativeWeight++
		}
	}

	if err := tree.Commit(); err != nil {
		return 0, iotago.Identifier{}, ierrors.Wrapf(err, "failed to commit attestation storage when committing slot %d", slot)
	}

	m.lastCommittedSlot = slot

	return m.lastCumulativeWeight, tree.Root(), nil
}

// Rollback rolls back the component state as if the last committed slot was targetSlot.
// It populates pendingAttestation store with previously committed attestations in order to create correct commitment in the future.
// As it modifies in-memory storage, it should only be called on the target engine as calling it on a temporary component will have no effect.
func (m *Manager) Rollback(targetSlot iotago.SlotIndex) error {
	m.commitmentMutex.RLock()
	defer m.commitmentMutex.RUnlock()

	if targetSlot > m.lastCommittedSlot {
		return ierrors.Errorf("slot %d is newer than last committed slot %d", targetSlot, m.lastCommittedSlot)
	}
	attestationSlot, isValid := m.computeAttestationCommitmentOffset(targetSlot)
	if !isValid {
		return nil
	}

	// We only need to export the committed attestations at targetSlot as these contain all the attestations for the
	// slots of targetSlot - attestationCommitmentOffset to targetSlot. This is sufficient to reconstruct the pending attestations
	// for targetSlot+1.
	attestationsStorage, err := m.attestationsForSlot(targetSlot)
	if err != nil {
		return ierrors.Wrapf(err, "failed to get attestations of slot %d", targetSlot)
	}

	if err = attestationsStorage.Stream(func(key iotago.AccountID, value *iotago.Attestation) error {
		m.applyToPendingAttestations(value, attestationSlot)

		return nil
	}); err != nil {
		return ierrors.Wrapf(err, "failed to stream attestations of slot %d", targetSlot)
	}

	return nil
}

func (m *Manager) computeAttestationCommitmentOffset(slot iotago.SlotIndex) (cutoffSlot iotago.SlotIndex, isValid bool) {
	if slot < m.apiProvider.APIForSlot(slot).ProtocolParameters().MaxCommittableAge() {
		return 0, false
	}

	return slot - m.apiProvider.APIForSlot(slot).ProtocolParameters().MaxCommittableAge(), true
}
