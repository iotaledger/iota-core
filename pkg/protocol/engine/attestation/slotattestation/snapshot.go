package slotattestation

import (
	"io"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/serializer/v2/stream"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (m *Manager) Import(reader io.ReadSeeker) error {
	m.commitmentMutex.Lock()
	defer m.commitmentMutex.Unlock()

	var attestations []*iotago.Attestation
	if err := stream.ReadCollection(reader, func(i int) error {
		attestationBytes, err := stream.ReadBlob(reader)
		if err != nil {
			return ierrors.Wrap(err, "failed to read attestation")
		}

		version, _, err := iotago.VersionFromBytes(attestationBytes)
		if err != nil {
			return ierrors.Wrap(err, "failed to determine version")
		}

		apiForVersion, err := m.apiProvider.APIForVersion(version)
		if err != nil {
			return ierrors.Wrapf(err, "failed to get API for version %d", version)
		}

		importedAttestation := new(iotago.Attestation)
		if _, err = apiForVersion.Decode(attestationBytes, importedAttestation); err != nil {
			return ierrors.Wrapf(err, "failed to decode attestation %d", i)
		}

		attestations = append(attestations, importedAttestation)

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

	m.TriggerInitialized()

	return nil
}

func (m *Manager) Export(writer io.WriteSeeker, targetSlot iotago.SlotIndex) error {
	m.commitmentMutex.RLock()
	defer m.commitmentMutex.RUnlock()

	if targetSlot > m.lastCommittedSlot {
		return ierrors.Errorf("slot %d is newer than last committed slot %d", targetSlot, m.lastCommittedSlot)
	}

	attestationSlotIndex, isValid := m.computeAttestationCommitmentOffset(targetSlot)
	if !isValid {
		if err := stream.Write(writer, uint64(0)); err != nil {
			return ierrors.Wrap(err, "failed to write 0 attestation count")
		}

		return nil
	}

	// We only need to export the committed attestations at targetSlot as these contain all the attestations for the
	// slots of targetSlot - attestationCommitmentOffset to targetSlot. This is sufficient to reconstruct the pending attestations
	// for targetSlot+1.
	attestationsStorage, err := m.attestationsForSlot(attestationSlotIndex)
	if err != nil {
		return ierrors.Wrapf(err, "failed to get attestations of slot %d", attestationSlotIndex)
	}

	var attestations []*iotago.Attestation
	if err = attestationsStorage.Stream(func(key iotago.AccountID, value *iotago.Attestation) error {
		attestations = append(attestations, value)

		return nil
	}); err != nil {
		return ierrors.Wrapf(err, "failed to stream attestations of slot %d", attestationSlotIndex)
	}

	if err = stream.WriteCollection(writer, func() (uint64, error) {
		for _, a := range attestations {
			apiForVersion, err := m.apiProvider.APIForVersion(a.ProtocolVersion)
			if err != nil {
				return 0, ierrors.Wrapf(err, "failed to get API for version %d", a.ProtocolVersion)
			}
			bytes, err := apiForVersion.Encode(a)
			if err != nil {
				return 0, ierrors.Wrapf(err, "failed to encode attestation %v", a)
			}

			if writeErr := stream.WriteBlob(writer, bytes); writeErr != nil {
				return 0, ierrors.Wrapf(writeErr, "failed to write attestation %v", a)
			}
		}

		return uint64(len(attestations)), nil
	}); err != nil {
		return ierrors.Wrapf(err, "failed to write attestations of slot %d", attestationSlotIndex)
	}

	return nil
}
