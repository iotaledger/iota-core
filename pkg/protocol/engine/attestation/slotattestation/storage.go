package slotattestation

import (
	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (m *Manager) RestoreFromDisk() error {
	m.commitmentMutex.Lock()
	defer m.commitmentMutex.Unlock()

	cutoffIndex, isValid := m.computeAttestationCommitmentOffset(m.lastCommittedSlot)

	for i := cutoffIndex; isValid && i <= m.lastCommittedSlot; i++ {
		storage, err := m.trackerStorage(i)
		if err != nil {
			return ierrors.Wrapf(err, "failed to get storage for slot %d", i)
		}

		//nolint:revive
		err = storage.Iterate(kvstore.EmptyPrefix, func(key iotago.AccountID, value *iotago.Attestation) bool {
			m.applyToPendingAttestations(value, cutoffIndex)
			return true
		})
		if err != nil {
			return ierrors.Wrapf(err, "failed to iterate over attestations of slot %d", i)
		}
		if err = storage.Clear(); err != nil {
			return ierrors.Wrapf(err, "failed to clear tracker attestations of slot %d", i)
		}
	}

	m.InitializedEvent().Trigger()

	return nil
}

func (m *Manager) writeToDisk() error {
	m.commitmentMutex.RLock()
	defer m.commitmentMutex.RUnlock()

	cutoffIndex, isValid := m.computeAttestationCommitmentOffset(m.lastCommittedSlot)
	if !isValid {
		return nil
	}

	for i := cutoffIndex; i <= m.lastCommittedSlot; i++ {
		storage, err := m.trackerStorage(i)
		if err != nil {
			return ierrors.Wrapf(err, "failed to get storage for slot %d", i)
		}

		attestations := m.determineAttestationsFromWindow(i)
		for _, a := range attestations {
			if err := storage.Set(a.Header.IssuerID, a); err != nil {
				return ierrors.Wrapf(err, "failed to set attestation %v", a)
			}
		}
	}

	return nil
}

func (m *Manager) trackerStorage(index iotago.SlotIndex) (*kvstore.TypedStore[iotago.AccountID, *iotago.Attestation], error) {
	trackerStorage, err := m.bucketedStorage(index)
	if err != nil {
		return nil, ierrors.Errorf("failed to access storage for tracker of slot %d", index)
	}
	trackerStorage, err = trackerStorage.WithExtendedRealm(kvstore.Realm{prefixAttestationsTracker})
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to get extended realm for tracker of slot %d", index)
	}

	return kvstore.NewTypedStore[iotago.AccountID, *iotago.Attestation](trackerStorage,
		iotago.AccountID.Bytes,
		iotago.AccountIDFromBytes,
		(*iotago.Attestation).Bytes,
		iotago.AttestationFromBytes(m.apiProvider),
	), nil
}

func (m *Manager) attestationsForSlot(index iotago.SlotIndex) (ads.Map[iotago.Identifier, iotago.AccountID, *iotago.Attestation], error) {
	attestationsStorage, err := m.bucketedStorage(index)
	if err != nil {
		return nil, ierrors.Errorf("failed to access storage for attestors of slot %d", index)
	}
	attestationsStorage, err = attestationsStorage.WithExtendedRealm(kvstore.Realm{prefixAttestationsADSMap})
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to get extended realm for attestations of slot %d", index)
	}

	return ads.NewMap[iotago.Identifier](
		attestationsStorage,
		iotago.Identifier.Bytes,
		iotago.IdentifierFromBytes,
		iotago.AccountID.Bytes,
		iotago.AccountIDFromBytes,
		(*iotago.Attestation).Bytes,
		iotago.AttestationFromBytes(m.apiProvider),
	), nil
}
