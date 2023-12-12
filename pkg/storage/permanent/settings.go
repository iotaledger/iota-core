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
	"github.com/iotaledger/hive.go/serializer/v2"
	"github.com/iotaledger/hive.go/serializer/v2/byteutils"
	"github.com/iotaledger/hive.go/serializer/v2/stream"
	"github.com/iotaledger/hive.go/stringify"
	"github.com/iotaledger/iota-core/pkg/model"
	iotago "github.com/iotaledger/iota.go/v4"
)

const (
	snapshotImportedKey = iota
	latestCommitmentKey
	latestFinalizedSlotKey
	latestStoredSlotKey
	latestNonEmptySlotKey
	protocolVersionEpochMappingKey
	futureProtocolParametersKey
	protocolParametersKey
	latestIssuedValidationBlock
)

type Settings struct {
	store                            kvstore.KVStore
	storeSnapshotImported            *kvstore.TypedValue[bool]
	storeLatestCommitment            *kvstore.TypedValue[*model.Commitment]
	storeLatestNonEmptySlot          *kvstore.TypedValue[iotago.SlotIndex]
	storeLatestFinalizedSlot         *kvstore.TypedValue[iotago.SlotIndex]
	storeLatestStoredSlot            *kvstore.TypedValue[iotago.SlotIndex]
	storeLatestIssuedValidationBlock *kvstore.TypedValue[*model.Block]

	mutex                            syncutils.RWMutex
	storeProtocolVersionEpochMapping *kvstore.TypedStore[iotago.Version, iotago.EpochIndex]
	storeFutureProtocolParameters    *kvstore.TypedStore[iotago.Version, *types.Tuple[iotago.EpochIndex, iotago.Identifier]]
	storeProtocolParameters          *kvstore.TypedStore[iotago.Version, iotago.ProtocolParameters]

	apiProvider *iotago.EpochBasedProvider
}

func NewSettings(store kvstore.KVStore, opts ...options.Option[iotago.EpochBasedProvider]) (settings *Settings) {
	apiProvider := iotago.NewEpochBasedProvider(opts...)

	s := &Settings{
		store:       store,
		apiProvider: apiProvider,
		storeSnapshotImported: kvstore.NewTypedValue(
			store,
			[]byte{snapshotImportedKey},
			func(v bool) ([]byte, error) {
				return []byte{lo.Cond[byte](v, 1, 0)}, nil
			},
			func(b []byte) (bool, int, error) {
				if len(b) != 1 {
					return false, 0, ierrors.Errorf("expected 1 byte, but got %d", len(b))
				}

				return b[0] == 1, 1, nil
			},
		),
		storeLatestCommitment: kvstore.NewTypedValue(
			store,
			[]byte{latestCommitmentKey},
			(*model.Commitment).Bytes,
			model.CommitmentFromBytes(apiProvider),
		),
		storeLatestFinalizedSlot: kvstore.NewTypedValue(
			store,
			[]byte{latestFinalizedSlotKey},
			iotago.SlotIndex.Bytes,
			iotago.SlotIndexFromBytes,
		),
		storeLatestStoredSlot: kvstore.NewTypedValue(
			store,
			[]byte{latestStoredSlotKey},
			iotago.SlotIndex.Bytes,
			iotago.SlotIndexFromBytes,
		),
		storeLatestNonEmptySlot: kvstore.NewTypedValue(
			store,
			[]byte{latestNonEmptySlotKey},
			iotago.SlotIndex.Bytes,
			iotago.SlotIndexFromBytes,
		),
		storeLatestIssuedValidationBlock: kvstore.NewTypedValue(
			store,
			[]byte{latestIssuedValidationBlock},
			(*model.Block).Bytes,
			model.BlockFromBytesFunc(apiProvider),
		),

		storeProtocolVersionEpochMapping: kvstore.NewTypedStore(
			lo.PanicOnErr(store.WithExtendedRealm([]byte{protocolVersionEpochMappingKey})),
			iotago.Version.Bytes,
			iotago.VersionFromBytes,
			iotago.EpochIndex.Bytes,
			iotago.EpochIndexFromBytes,
		),
		storeFutureProtocolParameters: kvstore.NewTypedStore(
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
		storeProtocolParameters: kvstore.NewTypedStore(
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
		s.apiProvider.SetCommittedSlot(s.latestCommitment().Slot())
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

func (s *Settings) APIProvider() *iotago.EpochBasedProvider {
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
	return lo.PanicOnErr(s.storeSnapshotImported.Has())
}

func (s *Settings) SetSnapshotImported() (err error) {
	return s.storeSnapshotImported.Set(true)
}

func (s *Settings) LatestCommitment() *model.Commitment {
	return s.latestCommitment()
}

func (s *Settings) SetLatestCommitment(latestCommitment *model.Commitment) (err error) {
	s.apiProvider.SetCommittedSlot(latestCommitment.Slot())

	// Delete the old future protocol parameters if they exist.
	_ = s.storeFutureProtocolParameters.Delete(s.apiProvider.VersionForSlot(latestCommitment.Slot()))

	return s.storeLatestCommitment.Set(latestCommitment)
}

func (s *Settings) latestCommitment() *model.Commitment {
	commitment, err := s.storeLatestCommitment.Get()
	if err != nil {
		if ierrors.Is(err, kvstore.ErrKeyNotFound) {
			return model.NewEmptyCommitment(s.apiProvider.CommittedAPI())
		}
		panic(err)
	}

	return commitment
}

func (s *Settings) SetLatestFinalizedSlot(slot iotago.SlotIndex) (err error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	return s.storeLatestFinalizedSlot.Set(slot)
}

func (s *Settings) LatestFinalizedSlot() iotago.SlotIndex {
	latestFinalizedSlot, err := s.storeLatestFinalizedSlot.Get()
	if err != nil {
		if ierrors.Is(err, kvstore.ErrKeyNotFound) {
			return s.apiProvider.CommittedAPI().ProtocolParameters().GenesisSlot()
		}
		panic(err)
	}

	return latestFinalizedSlot
}

func (s *Settings) LatestStoredSlot() iotago.SlotIndex {
	return read(s.storeLatestStoredSlot)
}

func (s *Settings) SetLatestStoredSlot(slot iotago.SlotIndex) (err error) {
	return s.storeLatestStoredSlot.Set(slot)
}

func (s *Settings) AdvanceLatestStoredSlot(slot iotago.SlotIndex) (err error) {
	// We don't need to advance the latest stored slot if it's already ahead of the given slot.
	// We check this before Compute to avoid contention inside the TypedValue.
	if s.LatestStoredSlot() >= slot {
		return nil
	}

	if _, err = s.storeLatestStoredSlot.Compute(func(latestStoredSlot iotago.SlotIndex, _ bool) (newValue iotago.SlotIndex, err error) {
		if latestStoredSlot >= slot {
			return latestStoredSlot, kvstore.ErrTypedValueNotChanged
		}

		return slot, nil
	}); err != nil {
		return ierrors.Wrap(err, "failed to advance latest stored slot")
	}

	return nil
}

func (s *Settings) LatestNonEmptySlot() iotago.SlotIndex {
	return read(s.storeLatestNonEmptySlot)
}

func (s *Settings) SetLatestNonEmptySlot(slot iotago.SlotIndex) (err error) {
	return s.storeLatestNonEmptySlot.Set(slot)
}

func (s *Settings) AdvanceLatestNonEmptySlot(slot iotago.SlotIndex) (err error) {
	if _, err = s.storeLatestNonEmptySlot.Compute(func(latestNonEmptySlot iotago.SlotIndex, _ bool) (newValue iotago.SlotIndex, err error) {
		if latestNonEmptySlot >= slot {
			return latestNonEmptySlot, kvstore.ErrTypedValueNotChanged
		}

		return slot, nil
	}); err != nil {
		return ierrors.Wrap(err, "failed to advance latest non-empty slot")
	}

	return nil
}

func (s *Settings) LatestIssuedValidationBlock() *model.Block {
	return read(s.storeLatestIssuedValidationBlock)
}

func (s *Settings) SetLatestIssuedValidationBlock(block *model.Block) (err error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	return s.storeLatestIssuedValidationBlock.Set(block)
}

func (s *Settings) Export(writer io.WriteSeeker, targetCommitment *iotago.Commitment) error {
	var commitmentBytes []byte
	var err error
	if targetCommitment != nil {
		// We always know the version of the target commitment, so there can be no error.
		commitmentBytes, err = lo.PanicOnErr(s.apiProvider.APIForVersion(targetCommitment.ProtocolVersion)).Encode(targetCommitment)
		if err != nil {
			return ierrors.Wrap(err, "failed to encode target commitment")
		}
	} else {
		commitmentBytes = s.LatestCommitment().Data()
	}

	if err := stream.WriteBytesWithSize(writer, commitmentBytes, serializer.SeriLengthPrefixTypeAsUint16); err != nil {
		return ierrors.Wrap(err, "failed to write commitment")
	}

	if err := stream.Write(writer, s.LatestFinalizedSlot()); err != nil {
		return ierrors.Wrap(err, "failed to write latest finalized slot")
	}

	s.mutex.RLock()
	defer s.mutex.RUnlock()

	// Export protocol versions
	if err := stream.WriteCollection(writer, serializer.SeriLengthPrefixTypeAsUint16, func() (int, error) {
		var count int
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

	// TODO: rollback future protocol parameters if it was added after targetCommitment.Slot()
	// Export future protocol parameters
	if err := stream.WriteCollection(writer, serializer.SeriLengthPrefixTypeAsUint16, func() (int, error) {
		var count int
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
	if err := stream.WriteCollection(writer, serializer.SeriLengthPrefixTypeAsUint16, func() (int, error) {
		var paramsCount int
		var innerErr error

		if err := s.storeProtocolParameters.KVStore().Iterate(kvstore.EmptyPrefix, func(key kvstore.Key, value kvstore.Value) bool {
			version, _, err := iotago.VersionFromBytes(key)
			if err != nil {
				innerErr = ierrors.Wrap(err, "failed to read version")
				return false
			}

			// We don't export future protocol parameters, just skip to the next ones.
			if s.apiProvider.CommittedAPI().Version() < version {
				return true
			}

			if err := stream.WriteBytesWithSize(writer, value, serializer.SeriLengthPrefixTypeAsUint32); err != nil {
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
	commitmentBytes, err := stream.ReadBytesWithSize(reader, serializer.SeriLengthPrefixTypeAsUint16)
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
	if err := stream.ReadCollection(reader, serializer.SeriLengthPrefixTypeAsUint16, func(i int) error {
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
	if err := stream.ReadCollection(reader, serializer.SeriLengthPrefixTypeAsUint16, func(i int) error {
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
	if err := stream.ReadCollection(reader, serializer.SeriLengthPrefixTypeAsUint16, func(i int) error {
		paramsBytes, err := stream.ReadBytesWithSize(reader, serializer.SeriLengthPrefixTypeAsUint32)
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
	commitment, err := lo.DropCount(model.CommitmentFromBytes(s.apiProvider)(commitmentBytes))
	if err != nil {
		return ierrors.Wrap(err, "failed to parse commitment")
	}

	if err := s.SetLatestCommitment(commitment); err != nil {
		return ierrors.Wrap(err, "failed to set latest commitment")
	}

	return nil
}

func (s *Settings) Rollback(targetCommitment *model.Commitment) error {
	// TODO: rollback future protocol parameters if it was added after targetCommitment.Slot()

	if err := s.SetLatestCommitment(targetCommitment); err != nil {
		return ierrors.Wrap(err, "failed to set latest commitment")
	}

	return nil
}

func (s *Settings) String() string {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	builder := stringify.NewStructBuilder("Settings")
	builder.AddField(stringify.NewStructField("IsSnapshotImported", lo.PanicOnErr(s.storeSnapshotImported.Has())))
	builder.AddField(stringify.NewStructField("LatestCommitment", s.latestCommitment()))
	builder.AddField(stringify.NewStructField("LatestFinalizedSlot", read(s.storeLatestFinalizedSlot)))
	if err := s.storeProtocolParameters.Iterate(kvstore.EmptyPrefix, func(version iotago.Version, protocolParams iotago.ProtocolParameters) bool {
		builder.AddField(stringify.NewStructField(fmt.Sprintf("ProtocolParameters(%d)", version), protocolParams))

		return true
	}); err != nil {
		panic(err)
	}

	return builder.String()
}

func read[T any](typedValue *kvstore.TypedValue[T]) (value T) {
	value, err := typedValue.Get()
	if err != nil && !ierrors.Is(err, kvstore.ErrKeyNotFound) {
		panic(err)
	}

	return value
}
