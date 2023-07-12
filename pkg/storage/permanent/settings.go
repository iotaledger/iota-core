package permanent

import (
	"fmt"
	"io"

	"golang.org/x/crypto/blake2b"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/syncutils"
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

	apiMutex         syncutils.RWMutex
	apiByVersion     map[iotago.Version]iotago.API
	protocolVersions []*protocolVersionEpochStart
}

func NewSettings(store kvstore.KVStore) (settings *Settings) {
	s := &Settings{
		store:        store,
		apiByVersion: make(map[iotago.Version]iotago.API),
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
	return s.APIForVersion(iotago.LatestProtocolVersion())
}

func (s *Settings) APIForSlot(slot iotago.SlotIndex) iotago.API {
	return s.APIForVersion(s.VersionForSlot(slot))
}

func (s *Settings) APIForEpoch(epoch iotago.EpochIndex) iotago.API {
	return s.APIForVersion(s.VersionForEpoch(epoch))
}

func (s *Settings) APIForVersion(version iotago.Version) iotago.API {
	s.apiMutex.RLock()
	if api, exists := s.apiByVersion[version]; exists {
		s.apiMutex.RUnlock()
		return api
	}
	s.apiMutex.RUnlock()

	api := s.apiFromProtocolParameters(version)

	s.apiMutex.Lock()
	s.apiByVersion[version] = api
	s.apiMutex.Unlock()

	return api
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
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	s.apiMutex.Lock()
	defer s.apiMutex.Unlock()

	if err := s.store.Iterate([]byte{protocolVersionEpochMappingKey}, func(key kvstore.Key, value kvstore.Value) bool {
		version, _, err := iotago.VersionFromBytes(key[1:])
		if err != nil {
			panic(err)
		}

		epoch, _, err := iotago.EpochIndexFromBytes(value)
		if err != nil {
			panic(err)
		}

		s.protocolVersions = append(s.protocolVersions, &protocolVersionEpochStart{
			Version:    version,
			StartEpoch: epoch,
		})

		return true
	}); err != nil {
		panic(err)
	}
}

func (s *Settings) StoreProtocolParametersEpochMapping(version iotago.Version, epoch iotago.EpochIndex) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.apiMutex.Lock()
	defer s.apiMutex.Unlock()

	bytes, err := epoch.Bytes()
	if err != nil {
		return err
	}

	if err := s.store.Set([]byte{protocolVersionEpochMappingKey, byte(version)}, bytes); err != nil {
		return err
	}

	s.protocolVersions = append(s.protocolVersions, &protocolVersionEpochStart{
		Version:    version,
		StartEpoch: epoch,
	})

	return nil
}

func (s *Settings) LatestCommitment() *model.Commitment {
	s.mutex.RLock()
	bytes, err := s.store.Get([]byte{latestCommitmentKey})
	s.mutex.RUnlock() // do not use defer because s.LatestAPI will also RLock the mutex

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

func (s *Settings) ProtocolParametersAndVersionsHash() ([32]byte, error) {
	// TODO: concatenate and hash protocol parameters and protocol versions
	// bytes, err := p.Bytes()
	// if err != nil {
	// 	return [32]byte{}, err
	// }
	return blake2b.Sum256([]byte{}), nil
}

func (s *Settings) VersionForEpoch(epoch iotago.EpochIndex) iotago.Version {
	s.apiMutex.RLock()
	defer s.apiMutex.RUnlock()

	for i := len(s.protocolVersions) - 1; i >= 0; i-- {
		if s.protocolVersions[i].StartEpoch <= epoch {
			return s.protocolVersions[i].Version
		}
	}

	// This means that the protocol versions are not properly configured.
	panic(ierrors.Errorf("could not find a protocol version for epoch %d", epoch))
}

func (s *Settings) VersionForSlot(slot iotago.SlotIndex) iotago.Version {
	return s.VersionForEpoch(s.LatestAPI().TimeProvider().EpochFromSlot(slot))
}

func (s *Settings) Export(writer io.WriteSeeker, targetCommitment *iotago.Commitment) error {
	var commitmentBytes []byte
	var err error
	if targetCommitment != nil {
		commitmentBytes, err = s.APIForVersion(targetCommitment.Version).Encode(targetCommitment)
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
		s.apiMutex.RLock()
		defer s.apiMutex.RUnlock()

		for _, versionEpochStart := range s.protocolVersions {
			if err := stream.Write(writer, versionEpochStart.Version); err != nil {
				return 0, ierrors.Wrap(err, "failed to encode version")
			}

			if err := stream.Write(writer, versionEpochStart.StartEpoch); err != nil {
				return 0, ierrors.Wrap(err, "failed to encode epoch")
			}
		}

		return uint64(len(s.protocolVersions)), nil
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

	var prevProtocolVersionEpochStart *protocolVersionEpochStart
	if err := stream.ReadCollection(reader, func(i int) error {
		version, err := stream.Read[iotago.Version](reader)
		if err != nil {
			return ierrors.Wrap(err, "failed to parse version")
		}

		epoch, err := stream.Read[iotago.EpochIndex](reader)
		if err != nil {
			return ierrors.Wrap(err, "failed to parse version")
		}

		current := &protocolVersionEpochStart{
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

func (s *Settings) latestAPI() iotago.API {
	return s.apiForVersion(iotago.LatestProtocolVersion())
}

func (s *Settings) latestCommitment() *model.Commitment {
	bytes, err := s.store.Get([]byte{latestCommitmentKey})

	if err != nil {
		if ierrors.Is(err, kvstore.ErrKeyNotFound) {
			return model.NewEmptyCommitment(s.latestAPI())
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

func (s *Settings) apiForVersion(version iotago.Version) iotago.API {
	if api, exists := s.apiByVersion[version]; exists {
		return api
	}

	api := s.apiFromProtocolParameters(version)

	s.apiMutex.Lock()
	s.apiByVersion[version] = api
	s.apiMutex.Unlock()

	return api
}

func (s *Settings) apiFromProtocolParameters(version iotago.Version) iotago.API {
	protocolParams := s.protocolParameters(version)
	if protocolParams == nil {
		panic(ierrors.Errorf("protocol parameters for version %d not found", version))
	}

	switch protocolParams.Version() {
	case 3:
		return iotago.V3API(protocolParams)
	default:
		panic(ierrors.Errorf("unsupported protocol version %d", protocolParams.Version()))
	}
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

type protocolVersionEpochStart struct {
	Version    iotago.Version
	StartEpoch iotago.EpochIndex
}
