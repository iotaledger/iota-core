package attestation

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

func (s *Tracker) TrackAttestation(attestation *iotago.Attestation) {
	var previousAttestationIndex iotago.SlotIndex
	var updated bool

	updatedAttestation := s.latestAttestations.Compute(attestation.IssuerID, func(currentValue *iotago.Attestation, exists bool) *iotago.Attestation {
		if !exists {
			return attestation
		}

		previousAttestationIndex = currentValue.SlotCommitmentID.Index()

		// Replace the stored attestation only if the new one is greater.
		if attestation.Compare(currentValue) == 1 {
			updated = true
			return attestation
		}

		return currentValue
	})

	// The new attestation is smaller or equal the previousAttestation. There's no need to update attestationsPerSlot.
	if !updated {
		return
	}

	for i := lo.Max(s.cutoffIndexCallback(), previousAttestationIndex) + 1; i <= updatedAttestation.SlotCommitmentID.Index(); i++ {
		s.attestationsPerSlot.Get(i, true).Set(attestation.IssuerID, updatedAttestation)
	}
}

func (s *Tracker) Attestations(slotIndex iotago.SlotIndex) []*iotago.Attestation {
	slotAttestors := s.attestationsPerSlot.Get(slotIndex, false)
	if slotAttestors == nil {
		return nil
	}

	return slotAttestors.Values()
}

func (s *Tracker) EvictSlot(indexToEvict iotago.SlotIndex) {
	slotAttestors := s.attestationsPerSlot.Evict(indexToEvict)
	if slotAttestors == nil {
		return
	}

	var attestorsToEvict []iotago.AccountID
	slotAttestors.ForEach(func(accountID iotago.AccountID, _ *iotago.Attestation) bool {
		latestAttestation, has := s.latestAttestations.Get(accountID)
		if !has {
			return true
		}
		if latestAttestation.SlotCommitmentID.Index() <= indexToEvict {
			attestorsToEvict = append(attestorsToEvict, accountID)
		}

		return true
	})

	for _, id := range attestorsToEvict {
		s.latestAttestations.Delete(id)
	}
}
