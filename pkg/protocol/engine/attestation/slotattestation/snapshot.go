package slotattestation

import (
	"io"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/serializer/v2"
	"github.com/iotaledger/hive.go/serializer/v2/stream"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (m *Manager) Import(reader io.ReadSeeker) error {
	m.commitmentMutex.Lock()
	defer m.commitmentMutex.Unlock()

	var attestations []*iotago.Attestation
	if err := stream.ReadCollection(reader, serializer.SeriLengthPrefixTypeAsUint32, func(i int) error {
		attestation, err := stream.ReadObjectWithSize[*iotago.Attestation](reader, serializer.SeriLengthPrefixTypeAsUint16, iotago.AttestationFromBytes(m.apiProvider))
		if err != nil {
			return ierrors.Wrapf(err, "failed to read attestation %d", i)
		}

		attestations = append(attestations, attestation)

		return nil
	}); err != nil {
		return ierrors.Wrapf(err, "failed to import attestations")
	}

	attestationSlotIndex, isValid := m.computeAttestationCommitmentOffset(m.lastCommittedSlot)
	if !isValid {
		return nil
	}

	for _, a := range attestations {
		m.applyToPendingAttestations(a, attestationSlotIndex)
	}

	m.InitializedEvent().Trigger()

	return nil
}

func (m *Manager) Export(writer io.WriteSeeker, targetSlot iotago.SlotIndex) error {
	m.commitmentMutex.RLock()
	defer m.commitmentMutex.RUnlock()

	if targetSlot > m.lastCommittedSlot {
		return ierrors.Errorf("slot %d is newer than last committed slot %d", targetSlot, m.lastCommittedSlot)
	}

	if _, isValid := m.computeAttestationCommitmentOffset(targetSlot); !isValid {
		if err := stream.WriteCollection(writer, serializer.SeriLengthPrefixTypeAsUint32, func() (int, error) {
			return 0, nil
		}); err != nil {
			return ierrors.Wrap(err, "failed to write 0 attestation count")
		}

		return nil
	}

	// We only need to export the committed attestations at targetSlot as these contain all the attestations for the
	// slots of targetSlot - attestationCommitmentOffset to targetSlot. This is sufficient to reconstruct the pending attestations
	// for targetSlot+1.
	attestationsStorage, err := m.attestationsForSlot(targetSlot)
	if err != nil {
		return ierrors.Wrapf(err, "failed to get attestations of slot %d", targetSlot)
	}

	var attestations []*iotago.Attestation
	//nolint:revive
	if err = attestationsStorage.Stream(func(key iotago.AccountID, value *iotago.Attestation) error {
		attestations = append(attestations, value)

		return nil
	}); err != nil {
		return ierrors.Wrapf(err, "failed to stream attestations of slot %d", targetSlot)
	}

	if err = stream.WriteCollection(writer, serializer.SeriLengthPrefixTypeAsUint32, func() (int, error) {
		for _, a := range attestations {
			if err := stream.WriteObjectWithSize(writer, a, serializer.SeriLengthPrefixTypeAsUint16, (*iotago.Attestation).Bytes); err != nil {
				return 0, ierrors.Wrapf(err, "failed to write attestation %v", a)
			}
		}

		return len(attestations), nil
	}); err != nil {
		return ierrors.Wrapf(err, "failed to write attestations of slot %d", targetSlot)
	}

	return nil
}
