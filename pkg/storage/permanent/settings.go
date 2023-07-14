package permanent

import (
	"fmt"
	"io"

	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/hive.go/serializer/v2/marshalutil"
	"github.com/iotaledger/hive.go/serializer/v2/stream"
	"github.com/iotaledger/hive.go/stringify"
	"github.com/iotaledger/iota-core/pkg/core/api"
	"github.com/iotaledger/iota-core/pkg/model"
	iotago "github.com/iotaledger/iota.go/v4"
)

const (
	snapshotImportedKey = iota
	latestCommitmentKey
	latestFinalizedSlotKey
	protocolParametersKey
	protocolVersionEpochMappingKey
)

type Settings struct {
	mutex syncutils.RWMutex
	store kvstore.KVStore

	apiByVersion     *shrinkingmap.ShrinkingMap[iotago.Version, iotago.API]
	protocolVersions *api.ProtocolEpochVersions
}

func NewSettings(store kvstore.KVStore) (settings *Settings) {
	s := &Settings{
		store:            store,
		apiByVersion:     shrinkingmap.New[iotago.Version, iotago.API](),
		protocolVersions: api.NewProtocolEpochVersions(),
	}

	s.loadProtocolParametersEpochMappings()

	return s
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

func (s *Settings) HighestSupportedProtocolVersion() iotago.Version {
	return iotago.LatestProtocolVersion()
}

var _ api.Provider = &Settings{}

func (s *Settings) LatestAPI() iotago.API {
	// There can't be an error since we are using the latest protocol version which should always exist.
	return lo.PanicOnErr(s.APIForVersion(iotago.LatestProtocolVersion()))
}

func (s *Settings) APIForSlot(slot iotago.SlotIndex) iotago.API {
	apiForSlot, err := s.APIForVersion(s.VersionForSlot(slot))

	// This means that the protocol version for the given slot is known but not supported yet. This could only happen after an upgrade
	// has been scheduled at a future epoch but the node software not yet upgraded to support it AND while misusing the API.
	// Before a slot can be determined APIForVersion must be called on said object which will already return an error that
	// the protocol version is not supported.
	if err != nil {
		panic(err)
	}

	return apiForSlot
}

func (s *Settings) APIForEpoch(epoch iotago.EpochIndex) iotago.API {
	apiForEpoch, err := s.APIForVersion(s.VersionForEpoch(epoch))

	// This means that the protocol version for the given epoch is known but not supported yet. This could only happen after an upgrade
	// has been scheduled at a future epoch but the node software not yet upgraded to support it AND while misusing the API.
	// Before a slot can be determined APIForVersion must be called on said object which will already return an error that
	// the protocol version is not supported.
	if err != nil {
		panic(err)
	}

	return apiForEpoch
}

// APIForVersion returns the API for the given protocol version.
// This function is safe to be called with data received from the network. It will return an error if the protocol
// version is not supported.
func (s *Settings) APIForVersion(version iotago.Version) (iotago.API, error) {
	// This is a hot path, so we first try to get (read only) the API from the map.
	if apiForVersion, exists := s.apiByVersion.Get(version); exists {
		return apiForVersion, nil
	}

	apiForVersion, err := s.apiFromProtocolParameters(version)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to create API from protocol parameters for version %d", version)
	}

	// If the API is not in the map, we need to create it and store it in the map in a safe way (might have been created by another goroutine in the meantime).
	apiForVersion, _ = s.apiByVersion.GetOrCreate(version, func() iotago.API {
		return apiForVersion
	})

	return apiForVersion, nil
}

func (s *Settings) StoreProtocolParameters(params iotago.ProtocolParameters) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	bytes, err := params.Bytes()
	if err != nil {
		return err
	}

	return s.store.Set([]byte{protocolParametersKey, byte(params.Version())}, bytes)
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

		s.protocolVersions.Add(version, epoch)

		return true
	}); err != nil {
		panic(err)
	}
}

func (s *Settings) StoreProtocolParametersEpochMapping(version iotago.Version, epoch iotago.EpochIndex) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	bytes, err := epoch.Bytes()
	if err != nil {
		return err
	}

	if err := s.store.Set([]byte{protocolVersionEpochMappingKey, byte(version)}, bytes); err != nil {
		return err
	}

	s.protocolVersions.Add(version, epoch)

	return nil
}

func (s *Settings) LatestCommitment() *model.Commitment {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	bytes, err := s.store.Get([]byte{latestCommitmentKey})

	if err != nil {
		if ierrors.Is(err, kvstore.ErrKeyNotFound) {
			return model.NewEmptyCommitment(s.LatestAPI())
		}
		panic(err)
	}

	return lo.PanicOnErr(model.CommitmentFromBytes(bytes, s))
}

func (s *Settings) SetLatestCommitment(latestCommitment *model.Commitment) (err error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	return s.store.Set([]byte{latestCommitmentKey}, latestCommitment.Data())
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

func (s *Settings) VersionsAndProtocolParametersHash(slot iotago.SlotIndex) (iotago.Identifier, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	currentEpoch := s.LatestAPI().TimeProvider().EpochFromSlot(slot)

	util := marshalutil.New()
	for _, version := range s.protocolVersions.Slice() {
		util.WriteBytes(lo.PanicOnErr(version.Version.Bytes()))
		util.WriteUint64(uint64(version.StartEpoch))

		// Only write the protocol parameters if the version is already active.
		// TODO: this is not optimal: we are not committing to the protocol parameters that are going to be active.
		if currentEpoch >= version.StartEpoch {
			paramsBytes, err := s.protocolParameters(version.Version).Bytes()
			if err != nil {
				return iotago.Identifier{}, ierrors.Wrap(err, "failed to get protocol parameters bytes")
			}
			util.WriteBytes(paramsBytes)
		}
	}

	return iotago.IdentifierFromData(util.Bytes()), nil
}

func (s *Settings) EpochForVersion(version iotago.Version) (iotago.EpochIndex, bool) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	return s.protocolVersions.EpochForVersion(version)
}

func (s *Settings) VersionForEpoch(epoch iotago.EpochIndex) iotago.Version {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	return s.protocolVersions.VersionForEpoch(epoch)
}

func (s *Settings) VersionForSlot(slot iotago.SlotIndex) iotago.Version {
	return s.VersionForEpoch(s.LatestAPI().TimeProvider().EpochFromSlot(slot))
}

func (s *Settings) Export(writer io.WriteSeeker, targetCommitment *iotago.Commitment) error {
	var commitmentBytes []byte
	var err error
	if targetCommitment != nil {
		// We always know the version of the target commitment, so there can be no error.
		commitmentBytes, err = lo.PanicOnErr(s.APIForVersion(targetCommitment.Version)).Encode(targetCommitment)
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

	if err := stream.WriteCollection(writer, func() (uint64, error) {
		s.mutex.RLock()
		defer s.mutex.RUnlock()

		protocolVersions := s.protocolVersions.Slice()
		for _, versionEpochStart := range protocolVersions {
			if err := stream.Write(writer, versionEpochStart.Version); err != nil {
				return 0, ierrors.Wrap(err, "failed to encode version")
			}

			if err := stream.Write(writer, versionEpochStart.StartEpoch); err != nil {
				return 0, ierrors.Wrap(err, "failed to encode epoch")
			}
		}

		return uint64(len(protocolVersions)), nil
	}); err != nil {
		return ierrors.Wrap(err, "failed to stream write protocol parameters")
	}

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

	var prevProtocolVersionEpochStart *api.ProtocolEpochVersion
	if err := stream.ReadCollection(reader, func(i int) error {
		version, err := stream.Read[iotago.Version](reader)
		if err != nil {
			return ierrors.Wrap(err, "failed to parse version")
		}

		epoch, err := stream.Read[iotago.EpochIndex](reader)
		if err != nil {
			return ierrors.Wrap(err, "failed to parse version")
		}

		current := &api.ProtocolEpochVersion{
			Version:    version,
			StartEpoch: epoch,
		}
		if prevProtocolVersionEpochStart != nil &&
			(current.Version <= prevProtocolVersionEpochStart.Version ||
				current.StartEpoch <= prevProtocolVersionEpochStart.StartEpoch) {
			panic(ierrors.Errorf("protocol versions not ordered correctly, %v is bigger than %v", prevProtocolVersionEpochStart, current))
		}

		// We also store the versions into the DB so that we can load it when we start the node from disk without loading a snapshot.
		if err := s.StoreProtocolParametersEpochMapping(version, epoch); err != nil {
			return ierrors.Wrap(err, "could not store protocol version epoch mapping")
		}

		prevProtocolVersionEpochStart = current

		return nil
	}); err != nil {
		return ierrors.Wrap(err, "failed to stream read protocol versions epoch mapping")
	}

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
	commitment, err := model.CommitmentFromBytes(commitmentBytes, s)
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
		builder.AddField(stringify.NewStructField(fmt.Sprintf("protocolParameters(%d)", key[1]), params))

		return true
	}); err != nil {
		panic(err)
	}

	return builder.String()
}

func (s *Settings) latestCommitment() *model.Commitment {
	bytes, err := s.store.Get([]byte{latestCommitmentKey})

	if err != nil {
		if ierrors.Is(err, kvstore.ErrKeyNotFound) {
			return model.NewEmptyCommitment(s.LatestAPI())
		}
		panic(err)
	}

	return lo.PanicOnErr(model.CommitmentFromBytes(bytes, s))
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

func (s *Settings) apiFromProtocolParameters(version iotago.Version) (iotago.API, error) {
	protocolParams := s.protocolParameters(version)
	if protocolParams == nil {
		return nil, ierrors.Errorf("protocol parameters for version %d not found", version)
	}

	switch protocolParams.Version() {
	case 3:
		return iotago.V3API(protocolParams), nil
	}

	return nil, ierrors.Errorf("unsupported protocol version %d", protocolParams.Version())
}

func (s *Settings) protocolParameters(version iotago.Version) iotago.ProtocolParameters {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	bytes, err := s.store.Get([]byte{protocolParametersKey, byte(version)})
	if err != nil {
		if ierrors.Is(err, kvstore.ErrKeyNotFound) {
			return nil
		}
		panic(err)
	}

	protocolParameters, _, err := iotago.ProtocolParametersFromBytes(bytes)
	if err != nil {
		panic(err)
	}

	return protocolParameters
}
