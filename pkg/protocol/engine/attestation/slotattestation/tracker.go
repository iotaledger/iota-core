package slotattestation

import (
	"github.com/iotaledger/hive.go/core/memstorage"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Tracker struct {
	attestationsPerSlot *memstorage.IndexedStorage[iotago.SlotIndex, iotago.AccountID, *iotago.Attestation]
}

func NewTracker() *Tracker {
	return &Tracker{
		attestationsPerSlot: memstorage.NewIndexedStorage[iotago.SlotIndex, iotago.AccountID, *iotago.Attestation](),
	}
}

func (t *Tracker) TrackAttestation(attestation *iotago.Attestation, cutoffIndex iotago.SlotIndex) {
	updated := true

	updatedAttestation := t.attestationsPerSlot.Get(cutoffIndex, true).Compute(attestation.IssuerID, func(currentValue *iotago.Attestation, exists bool) *iotago.Attestation {
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

	// The new attestation is smaller or equal the previousAttestation. There's no need to update attestationsPerSlot.
	if !updated {
		return
	}

	for i := cutoffIndex; i <= updatedAttestation.SlotCommitmentID.Index(); i++ {
		t.attestationsPerSlot.Get(i, true).Set(attestation.IssuerID, updatedAttestation)
	}
}

func (t *Tracker) Attestations(slotIndex iotago.SlotIndex) []*iotago.Attestation {
	slotAttestors := t.attestationsPerSlot.Get(slotIndex, false)
	if slotAttestors == nil {
		return nil
	}

	return slotAttestors.Values()
}

func (t *Tracker) EvictSlot(indexToEvict iotago.SlotIndex) []*iotago.Attestation {
	slotAttestors := t.attestationsPerSlot.Evict(indexToEvict)
	if slotAttestors == nil {
		return nil
	}

	return slotAttestors.Values()
}
