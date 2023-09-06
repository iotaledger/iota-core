package permanent

import (
	"fmt"
	"io"

	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/hive.go/serializer/v2/byteutils"
	"github.com/iotaledger/hive.go/serializer/v2/stream"
	"github.com/iotaledger/hive.go/stringify"
	"github.com/iotaledger/iota-core/pkg/model"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
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
	mutex                            syncutils.RWMutex
	store                            kvstore.KVStore
	storeProtocolVersionEpochMapping *kvstore.TypedStore[iotago.Version, iotago.EpochIndex]
	storeFutureProtocolParameters    *kvstore.TypedStore[iotago.Version, *types.Tuple[iotago.EpochIndex, iotago.Identifier]]
	storeProtocolParameters          *kvstore.TypedStore[iotago.Version, iotago.ProtocolParameters]

	apiProvider *api.EpochBasedProvider
}

func NewSettings(store kvstore.KVStore, opts ...options.Option[api.EpochBasedProvider]) (settings *Settings) {
	s := &Settings{
		store:       store,
		apiProvider: api.NewEpochBasedProvider(opts...),
		storeProtocolVersionEpochMapping: kvstore.NewTypedStore[iotago.Version, iotago.EpochIndex](
			lo.PanicOnErr(store.WithExtendedRealm([]byte{protocolVersionEpochMappingKey})),
			iotago.Version.Bytes,
			iotago.VersionFromBytes,
			iotago.EpochIndex.Bytes,
			iotago.EpochIndexFromBytes,
		),
		storeFutureProtocolParameters: kvstore.NewTypedStore[iotago.Version, *types.Tuple[iotago.EpochIndex, iotago.Identifier]](
			lo.PanicOnErr(store.WithExtendedRealm([]byte{futureProtocolParametersKey})),
			iotago.Version.Bytes,
			iotago.VersionFromBytes,
			func(t *types.Tuple[iotago.EpochIndex, iotago.Identifier]) ([]byte, error) {
				return byteutils.ConcatBytes(t.A.MustBytes(), lo.PanicOnErr(t.B.Bytes())), nil
			},
			func(b []byte) (*types.Tuple[iotago.EpochIndex, iotago.Identifier], int, error) {
				epoch, consumedBytes, err := iotago.EpochIndexFromBytes(b)
				if err != nil {
					return nil, 0, ierrors.Wrap(err, "failed to parse epoch index")
				}

				hash, consumedBytes2, err := iotago.IdentifierFromBytes(b[consumedBytes:])
				if err != nil {
					return nil, 0, ierrors.Wrap(err, "failed to parse identifier")
				}

				return types.NewTuple(epoch, hash), consumedBytes + consumedBytes2, nil
			},
		),
		storeProtocolParameters: kvstore.NewTypedStore[iotago.Version, iotago.ProtocolParameters](
			lo.PanicOnErr(store.WithExtendedRealm([]byte{protocolParametersKey})),
			iotago.Version.Bytes,
			iotago.VersionFromBytes,
			iotago.ProtocolParameters.Bytes,
			iotago.ProtocolParametersFromBytes,
		),
	}

	s.loadProtocolParameters()
	s.loadFutureProtocolParameters()
	s.loadProtocolParametersEpochMappings()
	if s.IsSnapshotImported() {
		s.apiProvider.SetCurrentSlot(s.latestCommitment().Index())
	}

	return s
}

func (s *Settings) loadProtocolParametersEpochMappings() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if err := s.storeProtocolVersionEpochMapping.Iterate(kvstore.EmptyPrefix, func(version iotago.Version, epoch iotago.EpochIndex) bool {
		s.apiProvider.AddVersion(version, epoch)

		return true
	}); err != nil {
		panic(err)
	}
}

func (s *Settings) loadProtocolParameters() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if err := s.storeProtocolParameters.Iterate(kvstore.EmptyPrefix, func(version iotago.Version, protocolParams iotago.ProtocolParameters) bool {
		s.apiProvider.AddProtocolParameters(protocolParams)

		return true
	}); err != nil {
		panic(err)
	}
}

func (s *Settings) loadFutureProtocolParameters() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if err := s.storeFutureProtocolParameters.Iterate(kvstore.EmptyPrefix, func(version iotago.Version, tuple *types.Tuple[iotago.EpochIndex, iotago.Identifier]) bool {
		s.apiProvider.AddFutureVersion(version, tuple.B, tuple.A)

		return true
	}); err != nil {
		panic(err)
	}
}

func (s *Settings) APIProvider() *api.EpochBasedProvider {
	return s.apiProvider
}

func (s *Settings) StoreProtocolParametersForStartEpoch(params iotago.ProtocolParameters, startEpoch iotago.EpochIndex) error {
	if err := s.StoreProtocolParameters(params); err != nil {
		return ierrors.Wrap(err, "failed to store protocol parameters")
	}

	if err := s.storeProtocolParametersEpochMapping(params.Version(), startEpoch); err != nil {
		return ierrors.Wrap(err, "failed to store protocol version epoch mapping")
	}

	return nil
}

func (s *Settings) StoreProtocolParameters(params iotago.ProtocolParameters) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if err := s.storeProtocolParameters.Set(params.Version(), params); err != nil {
		return ierrors.Wrap(err, "failed to store protocol parameters")
	}

	s.apiProvider.AddProtocolParameters(params)

	return nil
}

func (s *Settings) storeProtocolParametersEpochMapping(version iotago.Version, epoch iotago.EpochIndex) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if err := s.storeProtocolVersionEpochMapping.Set(version, epoch); err != nil {
		return ierrors.Wrap(err, "failed to store protocol version epoch mapping")
	}

	s.apiProvider.AddVersion(version, epoch)

	return nil
}

func (s *Settings) StoreFutureProtocolParametersHash(version iotago.Version, hash iotago.Identifier, epoch iotago.EpochIndex) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if err := s.storeFutureProtocolParameters.Set(version, types.NewTuple(epoch, hash)); err != nil {
		return ierrors.Wrap(err, "failed to store future protocol parameters")
	}

	if err := s.storeProtocolVersionEpochMapping.Set(version, epoch); err != nil {
		return ierrors.Wrap(err, "failed to store protocol version epoch mapping")
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

	// Delete the old future protocol parameters if they exist.
	_ = s.storeFutureProtocolParameters.Delete(s.apiProvider.VersionForSlot(latestCommitment.Index()))

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

	s.mutex.RLock()
	defer s.mutex.RUnlock()

	// Export protocol versions
	if err := stream.WriteCollection(writer, func() (uint64, error) {
		var count uint64
		var innerErr error

		if err := s.storeProtocolVersionEpochMapping.Iterate(kvstore.EmptyPrefix, func(version iotago.Version, epoch iotago.EpochIndex) bool {
			if err := stream.Write(writer, version); err != nil {
				innerErr = ierrors.Wrap(err, "failed to encode version")
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
		var count uint64
		var innerErr error

		if err := s.storeFutureProtocolParameters.Iterate(kvstore.EmptyPrefix, func(version iotago.Version, tuple *types.Tuple[iotago.EpochIndex, iotago.Identifier]) bool {
			if err := stream.Write(writer, version); err != nil {
				innerErr = ierrors.Wrap(err, "failed to encode version")
				return false
			}

			if err := stream.Write(writer, tuple.A); err != nil {
				innerErr = ierrors.Wrap(err, "failed to encode epoch")
				return false
			}

			if err := stream.Write(writer, tuple.B); err != nil {
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

	// Export protocol parameters: we only export the parameters up until the current active ones.
	if err := stream.WriteCollection(writer, func() (uint64, error) {
		var paramsCount uint64
		var innerErr error

		if err := s.storeProtocolParameters.KVStore().Iterate(kvstore.EmptyPrefix, func(key kvstore.Key, value kvstore.Value) bool {
			version, _, err := iotago.VersionFromBytes(key)
			if err != nil {
				innerErr = ierrors.Wrap(err, "failed to read version")
				return false
			}

			if s.apiProvider.IsFutureVersion(version) {
				// We don't export future protocol parameters, just skip to the next ones.
				return true
			}

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
	if err := s.storeProtocolParameters.Iterate(kvstore.EmptyPrefix, func(version iotago.Version, protocolParams iotago.ProtocolParameters) bool {
		builder.AddField(stringify.NewStructField(fmt.Sprintf("ProtocolParameters(%d)", version), protocolParams))

		return true
	}); err != nil {
		panic(err)
	}

	return builder.String()
}
