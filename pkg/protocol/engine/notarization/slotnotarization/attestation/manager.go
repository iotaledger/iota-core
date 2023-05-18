package attestation

import (
	"github.com/iotaledger/hive.go/core/memstorage"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Manager struct {
	pendingAttestations *memstorage.IndexedStorage[iotago.SlotIndex, iotago.AccountID, *iotago.Attestation]
}

func NewManager() *Manager {
	return &Manager{
		pendingAttestations: memstorage.NewIndexedStorage[iotago.SlotIndex, iotago.AccountID, *iotago.Attestation](),
	}
}

func (m *Manager) AddAttestationFromBlock(block *blocks.Block) {
	// TODO: check if blockIDIndex is above lastCommittedSlotIndex, otherwise ignore.

	attestation := iotago.NewAttestation(block.Block())

	storage := m.pendingAttestations.Get(block.ID().Index(), true)
	storage.Compute(block.Block().IssuerID, func(currentValue *iotago.Attestation, exists bool) *iotago.Attestation {
		if !exists {
			return attestation
		}

		// Replace the stored attestation only if the new one is greater.
		if attestation.Compare(currentValue) == 1 {
			return attestation
		}

		return currentValue
	})
}

func (m *Manager) Commit(index iotago.SlotIndex) {
	// TODO:
	//  1. apply all pending attestations of slot to the tracker
	//  2. remove all pending attestations of slot
	//  3. update lastCommittedSlotIndex
	//  4. get all attestations of slot - drift
	//  5. evict all attestations of slot - drift
	//  6. store all attestations of slot - drift in bucketed storage
	//  7. create ads map for all attestations of slot - drift
	//  8. calculate weight of all attestations of slot - drift
	//  9. get weight of slot before: current CW
	//  10. store CW + weight of all attestations of slot - drift in bucketed storage
}
