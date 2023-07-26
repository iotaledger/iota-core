package permanent

import (
	"fmt"
	"io"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/hive.go/serializer/v2/byteutils"
	"github.com/iotaledger/hive.go/serializer/v2/stream"
	"github.com/iotaledger/hive.go/stringify"
	"github.com/iotaledger/inx-app/pkg/api"
	"github.com/iotaledger/iota-core/pkg/model"
	iotago "github.com/iotaledger/iota.go/v4"
)

const (
	snapshotImportedKey = iota
	latestCommitmentKey
	latestFinalizedSlotKey
	protocolVersionEpochMappingKey
	futureProtocolParametersKey
	protocolParametersKey
)

type Settings struct {
	mutex syncutils.RWMutex
	store kvstore.KVStore

	apiProvider *api.EpochBasedProvider
}

func NewSettings(store kvstore.KVStore) (settings *Settings) {
	s := &Settings{
		store:       store,
		apiProvider: api.NewEpochBasedProvider(),
	}

	s.loadProtocolParametersEpochMappings()
	s.loadFutureProtocolParameters()
	s.loadProtocolParameters()
	if s.IsSnapshotImported() {
		s.apiProvider.SetCurrentSlot(s.latestCommitment().Index())
	}

	return s
}

func (s *Settings) loadProtocolParametersEpochMappings() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if err := s.store.Iterate([]byte{protocolVersionEpochMappingKey}, func(key kvstore.Key, value kvstore.Value) bool {
		version, _, err := iotago.VersionFromBytes(key[1:])
		if err != nil {
			panic(err)
		}

		epoch, _, err := iotago.EpochIndexFromBytes(value)
		if err != nil {
			panic(err)
		}

		s.apiProvider.AddVersion(version, epoch)

		return true
	}); err != nil {
		panic(err)
	}
}

func (s *Settings) loadProtocolParameters() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if err := s.store.Iterate([]byte{protocolParametersKey}, func(key kvstore.Key, value kvstore.Value) bool {
		protocolParams, _, err := iotago.ProtocolParametersFromBytes(value)
		if err != nil {
			return true // skip over invalid protocol parameters
		}

		s.apiProvider.AddProtocolParameters(protocolParams)

		return true
	}); err != nil {
		panic(err)
	}
}

func (s *Settings) loadFutureProtocolParameters() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if err := s.store.Iterate([]byte{futureProtocolParametersKey}, func(key kvstore.Key, value kvstore.Value) bool {
		version, _, err := iotago.VersionFromBytes(key[1:])
		if err != nil {
			panic(err)
		}

		epoch, read, err := iotago.EpochIndexFromBytes(value)
		if err != nil {
			panic(err)
		}

		hash, _, err := iotago.IdentifierFromBytes(value[read:])
		if err != nil {
			panic(err)
		}

		s.apiProvider.AddFutureVersion(version, hash, epoch)

		return true
	}); err != nil {
		panic(err)
	}
}

func (s *Settings) APIProvider() *api.EpochBasedProvider {
	return s.apiProvider
}

func (s *Settings) StoreProtocolParametersForStartEpoch(params iotago.ProtocolParameters, startEpoch iotago.EpochIndex) error {
	if err := s.storeProtocolParametersEpochMapping(params.Version(), startEpoch); err != nil {
		return err
	}

	return s.StoreProtocolParameters(params)
}

func (s *Settings) StoreProtocolParameters(params iotago.ProtocolParameters) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	bytes, err := params.Bytes()
	if err != nil {
		return err
	}

	if err := s.store.Set([]byte{protocolParametersKey, byte(params.Version())}, bytes); err != nil {
		return err
	}

	s.apiProvider.AddProtocolParameters(params)

	// Delete the old future protocol parameters if they exist.
	_ = s.store.Delete([]byte{futureProtocolParametersKey, byte(params.Version())})

	return nil
}

func (s *Settings) storeProtocolParametersEpochMapping(version iotago.Version, epoch iotago.EpochIndex) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	epochBytes, err := epoch.Bytes()
	if err != nil {
		return err
	}

	if err := s.store.Set([]byte{protocolVersionEpochMappingKey, byte(version)}, epochBytes); err != nil {
		return err
	}

	s.apiProvider.AddVersion(version, epoch)

	return nil
}

func (s *Settings) StoreFutureProtocolParametersHash(version iotago.Version, hash iotago.Identifier, epoch iotago.EpochIndex) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	epochBytes, err := epoch.Bytes()
	if err != nil {
		return err
	}

	hashBytes, err := hash.Bytes()
	if err != nil {
		return err
	}

	if err := s.store.Set([]byte{futureProtocolParametersKey, byte(version)}, byteutils.ConcatBytes(epochBytes, hashBytes)); err != nil {
		return err
	}

	s.apiProvider.AddFutureVersion(version, hash, epoch)

	return nil
}

func (s *Settings) IsSnapshotImported() bool {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	return lo.PanicOnErr(s.store.Has([]byte{snapshotImportedKey}))
}

func (s *Settings) SetSnapshotImported() (err error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	return s.store.Set([]byte{snapshotImportedKey}, []byte{1})
}

func (s *Settings) LatestCommitment() *model.Commitment {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	return s.latestCommitment()
}

func (s *Settings) SetLatestCommitment(latestCommitment *model.Commitment) (err error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.apiProvider.SetCurrentSlot(latestCommitment.Index())

	return s.store.Set([]byte{latestCommitmentKey}, latestCommitment.Data())
}

func (s *Settings) latestCommitment() *model.Commitment {
	bytes, err := s.store.Get([]byte{latestCommitmentKey})

	if err != nil {
		if ierrors.Is(err, kvstore.ErrKeyNotFound) {
			return model.NewEmptyCommitment(s.apiProvider.CurrentAPI())
		}
		panic(err)
	}

	return lo.PanicOnErr(model.CommitmentFromBytes(bytes, s.apiProvider))
}

func (s *Settings) LatestFinalizedSlot() iotago.SlotIndex {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	return s.latestFinalizedSlot()
}

func (s *Settings) SetLatestFinalizedSlot(index iotago.SlotIndex) (err error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	return s.store.Set([]byte{latestFinalizedSlotKey}, index.MustBytes())
}

func (s *Settings) latestFinalizedSlot() iotago.SlotIndex {
	bytes, err := s.store.Get([]byte{latestFinalizedSlotKey})
	if err != nil {
		if ierrors.Is(err, kvstore.ErrKeyNotFound) {
			return 0
		}
		panic(err)
	}
	i, _, err := iotago.SlotIndexFromBytes(bytes)
	if err != nil {
		panic(err)
	}

	return i
}

func (s *Settings) Export(writer io.WriteSeeker, targetCommitment *iotago.Commitment) error {
	var commitmentBytes []byte
	var err error
	if targetCommitment != nil {
		// We always know the version of the target commitment, so there can be no error.
		commitmentBytes, err = lo.PanicOnErr(s.apiProvider.APIForVersion(targetCommitment.Version)).Encode(targetCommitment)
		if err != nil {
			return ierrors.Wrap(err, "failed to encode target commitment")
		}
	} else {
		commitmentBytes = s.LatestCommitment().Data()
	}

	if err := stream.WriteBlob(writer, commitmentBytes); err != nil {
		return ierrors.Wrap(err, "failed to write commitment")
	}

	if err := stream.Write(writer, s.LatestFinalizedSlot()); err != nil {
		return ierrors.Wrap(err, "failed to write latest finalized slot")
	}

	// Export protocol versions
	if err := stream.WriteCollection(writer, func() (uint64, error) {
		s.mutex.RLock()
		defer s.mutex.RUnlock()

		var count uint64
		var innerErr error
		if err := s.store.Iterate([]byte{protocolVersionEpochMappingKey}, func(key kvstore.Key, value kvstore.Value) bool {
			version, _, err := iotago.VersionFromBytes(key[1:])
			if err != nil {
				innerErr = ierrors.Wrap(err, "failed to read version")
				return false
			}

			if err := stream.Write(writer, version); err != nil {
				innerErr = ierrors.Wrap(err, "failed to encode version")
				return false
			}

			epoch, _, err := iotago.EpochIndexFromBytes(value)
			if err != nil {
				innerErr = ierrors.Wrap(err, "failed to encode epoch")
				return false
			}

			if err := stream.Write(writer, epoch); err != nil {
				innerErr = ierrors.Wrap(err, "failed to encode epoch")
				return false
			}

			count++

			return true
		}); err != nil {
			return 0, ierrors.Wrap(err, "failed to iterate over protocol version epoch mapping")
		}
		if innerErr != nil {
			return 0, ierrors.Wrap(innerErr, "failed to write protocol version epoch mapping")
		}

		return count, nil
	}); err != nil {
		return ierrors.Wrap(err, "failed to stream write protocol version epoch mapping")
	}

	// Export future protocol parameters
	if err := stream.WriteCollection(writer, func() (uint64, error) {
		s.mutex.RLock()
		defer s.mutex.RUnlock()

		var count uint64
		var innerErr error
		if err := s.store.Iterate([]byte{futureProtocolParametersKey}, func(key kvstore.Key, value kvstore.Value) bool {
			version, _, err := iotago.VersionFromBytes(key[1:])
			if err != nil {
				innerErr = ierrors.Wrap(err, "failed to read version")
				return false
			}

			if err := stream.Write(writer, version); err != nil {
				innerErr = ierrors.Wrap(err, "failed to encode version")
				return false
			}

			epoch, read, err := iotago.EpochIndexFromBytes(value)
			if err != nil {
				innerErr = ierrors.Wrap(err, "failed to encode epoch")
				return false
			}

			if err := stream.Write(writer, epoch); err != nil {
				innerErr = ierrors.Wrap(err, "failed to encode epoch")
				return false
			}

			hash, _, err := iotago.IdentifierFromBytes(value[read:])
			if err != nil {
				innerErr = ierrors.Wrap(err, "failed to read hash")
				return false
			}

			if err := stream.Write(writer, hash); err != nil {
				innerErr = ierrors.Wrap(err, "failed to encode hash")
				return false
			}

			count++

			return true
		}); err != nil {
			return 0, ierrors.Wrap(err, "failed to iterate over future protocol parameters")
		}
		if innerErr != nil {
			return 0, ierrors.Wrap(innerErr, "failed to write future protocol parameters")
		}

		return count, nil
	}); err != nil {
		return ierrors.Wrap(err, "failed to stream write future protocol parameters")
	}

	// Export protocol parameters
	if err := stream.WriteCollection(writer, func() (uint64, error) {
		s.mutex.RLock()
		defer s.mutex.RUnlock()

		var paramsCount uint64
		var innerErr error
		if err := s.store.Iterate([]byte{protocolParametersKey}, func(_ kvstore.Key, value kvstore.Value) bool {
			if err := stream.WriteBlob(writer, value); err != nil {
				innerErr = err
				return false
			}
			paramsCount++

			return true
		}); err != nil {
			return 0, ierrors.Wrap(err, "failed to iterate over protocol parameters")
		}
		if innerErr != nil {
			return 0, ierrors.Wrap(innerErr, "failed to write protocol parameters")
		}

		return paramsCount, nil
	}); err != nil {
		return ierrors.Wrap(err, "failed to stream write protocol parameters")
	}

	return nil
}

func (s *Settings) Import(reader io.ReadSeeker) (err error) {
	commitmentBytes, err := stream.ReadBlob(reader)
	if err != nil {
		return ierrors.Wrap(err, "failed to read commitment")
	}

	latestFinalizedSlot, err := stream.Read[iotago.SlotIndex](reader)
	if err != nil {
		return ierrors.Wrap(err, "failed to read latest finalized slot")
	}

	if err := s.SetLatestFinalizedSlot(latestFinalizedSlot); err != nil {
		return ierrors.Wrap(err, "failed to set latest finalized slot")
	}

	// Read protocol version epoch mapping
	if err := stream.ReadCollection(reader, func(i int) error {
		version, err := stream.Read[iotago.Version](reader)
		if err != nil {
			return ierrors.Wrap(err, "failed to parse version")
		}

		epoch, err := stream.Read[iotago.EpochIndex](reader)
		if err != nil {
			return ierrors.Wrap(err, "failed to parse version")
		}

		// We also store the versions into the DB so that we can load it when we start the node from disk without loading a snapshot.
		if err := s.storeProtocolParametersEpochMapping(version, epoch); err != nil {
			return ierrors.Wrap(err, "could not store protocol version epoch mapping")
		}

		return nil
	}); err != nil {
		return ierrors.Wrap(err, "failed to stream read protocol versions epoch mapping")
	}

	// Read future protocol parameters
	if err := stream.ReadCollection(reader, func(i int) error {
		version, err := stream.Read[iotago.Version](reader)
		if err != nil {
			return ierrors.Wrap(err, "failed to parse version")
		}

		epoch, err := stream.Read[iotago.EpochIndex](reader)
		if err != nil {
			return ierrors.Wrap(err, "failed to parse version")
		}

		hash, err := stream.Read[iotago.Identifier](reader)
		if err != nil {
			return ierrors.Wrap(err, "failed to parse hash")
		}

		if err := s.StoreFutureProtocolParametersHash(version, hash, epoch); err != nil {
			return ierrors.Wrap(err, "could not store protocol version epoch mapping")
		}

		return nil
	}); err != nil {
		return ierrors.Wrap(err, "failed to stream read protocol versions epoch mapping")
	}

	// Read protocol parameters
	if err := stream.ReadCollection(reader, func(i int) error {
		paramsBytes, err := stream.ReadBlob(reader)
		if err != nil {
			return ierrors.Wrapf(err, "failed to read protocol parameters bytes at index %d", i)
		}
		params, _, err := iotago.ProtocolParametersFromBytes(paramsBytes)
		if err != nil {
			return ierrors.Wrapf(err, "failed to parse protocol parameters at index %d", i)
		}

		if err := s.StoreProtocolParameters(params); err != nil {
			return ierrors.Wrapf(err, "failed to store protocol parameters at index %d", i)
		}

		return nil
	}); err != nil {
		return ierrors.Wrap(err, "failed to stream read protocol parameters")
	}

	// Now that we parsed the protocol parameters, we can parse the commitment since there will be an API available
	commitment, err := model.CommitmentFromBytes(commitmentBytes, s.apiProvider)
	if err != nil {
		return ierrors.Wrap(err, "failed to parse commitment")
	}

	if err := s.SetLatestCommitment(commitment); err != nil {
		return ierrors.Wrap(err, "failed to set latest commitment")
	}

	return nil
}

func (s *Settings) String() string {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	builder := stringify.NewStructBuilder("Settings")
	builder.AddField(stringify.NewStructField("IsSnapshotImported", lo.PanicOnErr(s.store.Has([]byte{snapshotImportedKey}))))
	builder.AddField(stringify.NewStructField("LatestCommitment", s.latestCommitment()))
	builder.AddField(stringify.NewStructField("LatestFinalizedSlot", s.latestFinalizedSlot()))
	if err := s.store.Iterate([]byte{protocolParametersKey}, func(key kvstore.Key, value kvstore.Value) bool {
		params, _, err := iotago.ProtocolParametersFromBytes(value)
		if err != nil {
			panic(err)
		}
		builder.AddField(stringify.NewStructField(fmt.Sprintf("ProtocolParameters(%d)", key[1]), params))

		return true
	}); err != nil {
		panic(err)
	}

	return builder.String()
}
