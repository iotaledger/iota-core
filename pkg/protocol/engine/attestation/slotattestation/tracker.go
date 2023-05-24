package slotattestation

import (
	"github.com/iotaledger/hive.go/core/memstorage"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Tracker struct {
	latestAttestations  *shrinkingmap.ShrinkingMap[iotago.AccountID, *iotago.Attestation]
	attestationsPerSlot *memstorage.IndexedStorage[iotago.SlotIndex, iotago.AccountID, *iotago.Attestation]

	cutoffIndexCallback func() iotago.SlotIndex
}

func NewTracker(cutoffIndexCallback func() iotago.SlotIndex) *Tracker {
	return &Tracker{
		latestAttestations:  shrinkingmap.New[iotago.AccountID, *iotago.Attestation](),
		attestationsPerSlot: memstorage.NewIndexedStorage[iotago.SlotIndex, iotago.AccountID, *iotago.Attestation](),

		cutoffIndexCallback: cutoffIndexCallback,
	}
}

func (t *Tracker) TrackAttestation(attestation *iotago.Attestation) {
	var previousAttestationIndex iotago.SlotIndex
	updated := true

	updatedAttestation := t.latestAttestations.Compute(attestation.IssuerID, func(currentValue *iotago.Attestation, exists bool) *iotago.Attestation {
		if !exists {
			return attestation
		}

		previousAttestationIndex = currentValue.SlotCommitmentID.Index()

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

	for i := lo.Max(t.cutoffIndexCallback(), previousAttestationIndex); i <= updatedAttestation.SlotCommitmentID.Index(); i++ {
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

	var attestorsToEvict []iotago.AccountID
	slotAttestors.ForEach(func(accountID iotago.AccountID, _ *iotago.Attestation) bool {
		latestAttestation, has := t.latestAttestations.Get(accountID)
		if !has {
			return true
		}
		if latestAttestation.SlotCommitmentID.Index() <= indexToEvict {
			attestorsToEvict = append(attestorsToEvict, accountID)
		}

		return true
	})

	for _, id := range attestorsToEvict {
		t.latestAttestations.Delete(id)
	}

	return slotAttestors.Values()
}
