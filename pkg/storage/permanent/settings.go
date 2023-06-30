package permanent

import (
	"encoding/binary"
	"fmt"
	"io"
	"sync"

	"github.com/pkg/errors"
	"golang.org/x/crypto/blake2b"

	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/serializer/v2/stream"
	"github.com/iotaledger/hive.go/stringify"
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

type APIByVersionProviderFunc func(byte) iotago.API
type APIBySlotIndexProviderFunc func(iotago.SlotIndex) iotago.API

type Settings struct {
	mutex sync.RWMutex
	store kvstore.KVStore

	apiByVersion     map[iotago.Version]iotago.API
	protocolVersions []*protocolVersionEpochStart
}

func NewSettings(store kvstore.KVStore) (settings *Settings) {
	s := &Settings{
		store: store,
	}

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

func (s *Settings) LatestAPI() iotago.API {
	return s.API(iotago.LatestProtocolVersion())
}

func (s *Settings) APIForSlot(slot iotago.SlotIndex) iotago.API {
	return s.API(s.VersionForSlot(slot))
}

func (s *Settings) API(version iotago.Version) iotago.API {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	if api, exists := s.apiByVersion[version]; exists {
		return api
	}

	protocolParams := s.protocolParameters(version)

	if protocolParams == nil {
		panic(fmt.Errorf("protocol parameters for version %d not found", version))
	}

	// TODO: we need to double lock here as we're writing and obtain only the read lock above
	var api iotago.API
	switch protocolParams.Version() {
	case 3:
		api = iotago.V3API(protocolParams)
		s.apiByVersion[3] = api

		return api
	default:
		panic(fmt.Errorf("unsupported protocol version %d", protocolParams.Version()))
	}
}

func (s *Settings) protocolParameters(version iotago.Version) iotago.ProtocolParameters {
	bytes, err := s.store.Get([]byte{protocolParametersKey, version})
	if err != nil {
		if errors.Is(err, kvstore.ErrKeyNotFound) {
			return nil
		}
		panic(err)
	}

	commitment, _, err := iotago.ProtocolParametersFromBytes(bytes)
	if err != nil {
		panic(err)
	}

	return commitment
}

func (s *Settings) StoreProtocolParameters(params iotago.ProtocolParameters) (err error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	bytes, err := params.Bytes()
	if err != nil {
		return err
	}

	return s.store.Set([]byte{protocolParametersKey, params.Version()}, bytes)
}

func (s *Settings) loadProtocolParametersEpochMapping(version iotago.Version) *protocolVersionEpochStart {
	bytes, err := s.store.Get([]byte{protocolVersionEpochMappingKey, version})
	if err != nil {
		if errors.Is(err, kvstore.ErrKeyNotFound) {
			return nil
		}
		panic(err)
	}

	epoch, _, err := iotago.EpochIndexFromBytes(bytes)
	if err != nil {
		panic(err)
	}

	return &protocolVersionEpochStart{
		Version:    version,
		StartEpoch: epoch,
	}
}

func (s *Settings) storeProtocolParametersEpochMapping(version iotago.Version, epoch iotago.EpochIndex) (err error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	bytes, err := epoch.Bytes()
	if err != nil {
		return err
	}

	return s.store.Set([]byte{protocolParametersKey, version}, bytes)
}

func (s *Settings) LatestCommitment() *model.Commitment {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	bytes, err := s.store.Get([]byte{latestCommitmentKey})
	if err != nil {
		if errors.Is(err, kvstore.ErrKeyNotFound) {
			return model.NewEmptyCommitment(s.LatestAPI())
		}
		panic(err)
	}

	return lo.PanicOnErr(model.CommitmentFromBytes(bytes, s.LatestAPI()))
}

func (s *Settings) SetLatestCommitment(latestCommitment *model.Commitment) (err error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	return s.store.Set([]byte{latestCommitmentKey}, latestCommitment.Data())
}

func (s *Settings) LatestFinalizedSlot() iotago.SlotIndex {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	bytes, err := s.store.Get([]byte{latestFinalizedSlotKey})
	if err != nil {
		if errors.Is(err, kvstore.ErrKeyNotFound) {
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
	for i := len(s.protocolVersions) - 1; i >= 0; i-- {
		if s.protocolVersions[i].StartEpoch <= epoch {
			return s.protocolVersions[i].Version
		}
	}

	// This means that the protocol versions are not properly configured.
	panic(fmt.Sprintf("could not find a protocol version for epoch %d", epoch))
}

func (s *Settings) VersionForSlot(slot iotago.SlotIndex) iotago.Version {
	return s.VersionForEpoch(s.APIForSlot(slot).TimeProvider().EpochFromSlot(slot))
}

func (s *Settings) Export(writer io.WriteSeeker, targetCommitment *iotago.Commitment) (err error) {
	var commitmentBytes []byte
	if targetCommitment != nil {
		commitmentBytes, err = s.API(targetCommitment.Version).Encode(targetCommitment)
		if err != nil {
			return errors.Wrap(err, "failed to encode target commitment")
		}
	} else {
		commitmentBytes = s.LatestCommitment().Data()
	}
	if err = binary.Write(writer, binary.LittleEndian, uint32(len(commitmentBytes))); err != nil {
		return errors.Wrap(err, "failed to write commitment length")
	}
	if _, err := writer.Write(commitmentBytes); err != nil {
		return errors.Wrap(err, "failed to write commitment bytes")
	}

	finalizedSlotBytes, err := s.LatestFinalizedSlot().Bytes()
	if err != nil {
		return errors.Wrap(err, "failed to encode latest finalized slot")
	}
	if _, err = writer.Write(finalizedSlotBytes); err != nil {
		return errors.Wrap(err, "failed to write latest finalized slot")
	}

	if err := stream.WriteCollection(writer, func() (uint64, error) {
		for _, versionEpochStart := range s.protocolVersions {
			if err := stream.Write(writer, versionEpochStart.Version); err != nil {
				return 0, errors.Wrap(err, "failed to encode version")
			}

			if err := stream.Write(writer, versionEpochStart.StartEpoch); err != nil {
				return 0, errors.Wrap(err, "failed to encode epoch")
			}
		}

		return uint64(len(s.protocolVersions)), nil
	}); err != nil {
		return errors.Wrap(err, "failed to stream write protocol parameters")
	}

	if err := stream.WriteCollection(writer, func() (uint64, error) {
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
			return 0, errors.Wrap(err, "failed to iterate over protocol parameters")
		}
		if innerErr != nil {
			return 0, errors.Wrap(innerErr, "failed to write protocol parameters")
		}

		return paramsCount, nil
	}); err != nil {
		return errors.Wrap(err, "failed to stream write protocol parameters")
	}

	return nil
}

func (s *Settings) Import(reader io.ReadSeeker) (err error) {
	var commitmentLength uint32
	if err = binary.Read(reader, binary.LittleEndian, &commitmentLength); err != nil {
		return errors.Wrap(err, "failed to read commitment length")
	}

	commitmentBytes := make([]byte, commitmentLength)
	if err = binary.Read(reader, binary.LittleEndian, commitmentBytes); err != nil {
		return errors.Wrap(err, "failed to read commitment bytes")
	}

	var latestFinalizedSlotBytes [iotago.SlotIdentifierLength]byte
	if err = binary.Read(reader, binary.LittleEndian, latestFinalizedSlotBytes); err != nil {
		return errors.Wrap(err, "failed to read latest finalized slot bytes")
	}
	slotIndex, _, err := iotago.SlotIndexFromBytes(latestFinalizedSlotBytes[:])
	if err != nil {
		return errors.Wrap(err, "failed to parse latest finalized slot")
	}
	if err := s.SetLatestFinalizedSlot(slotIndex); err != nil {
		return errors.Wrap(err, "failed to set latest finalized slot")
	}

	var prevProtocolVersionEpochStart *protocolVersionEpochStart
	if err := stream.ReadCollection(reader, func(i int) error {
		version, err := stream.Read[iotago.Version](reader)
		if err != nil {
			return errors.Wrap(err, "failed to parse version")
		}

		epoch, err := stream.Read[iotago.EpochIndex](reader)
		if err != nil {
			return errors.Wrap(err, "failed to parse version")
		}

		current := &protocolVersionEpochStart{
			Version:    version,
			StartEpoch: epoch,
		}
		if prevProtocolVersionEpochStart != nil &&
			(current.Version <= prevProtocolVersionEpochStart.Version ||
				current.StartEpoch <= prevProtocolVersionEpochStart.StartEpoch) {
			panic(fmt.Sprintf("protocol versions not ordered correctly, %v is bigger than %v", prevProtocolVersionEpochStart, current))
		}

		// We store the versions into a sorted slice to be able to look up the versions quickly from memory.
		s.protocolVersions = append(s.protocolVersions, current)
		// We also store the versions into the DB so that we can load it when we start the node from disk without loading a snapshot.
		if err := s.storeProtocolParametersEpochMapping(version, epoch); err != nil {
			return errors.Wrap(err, "could not store protocol version epoch mapping")
		}

		prevProtocolVersionEpochStart = current

		return nil
	}); err != nil {
		return errors.Wrap(err, "failed to stream read protocol versions epoch mapping")
	}

	if err := stream.ReadCollection(reader, func(i int) error {
		paramsBytes, err := stream.ReadBlob(reader)
		if err != nil {
			return errors.Wrapf(err, "failed to read protocol parameters bytes at index %d", i)
		}
		params, _, err := iotago.ProtocolParametersFromBytes(paramsBytes)
		if err != nil {
			return errors.Wrapf(err, "failed to parse protocol parameters at index %d", i)
		}

		if err := s.StoreProtocolParameters(params); err != nil {
			return errors.Wrapf(err, "failed to store protocol parameters at index %d", i)
		}

		return nil
	}); err != nil {
		return errors.Wrap(err, "failed to stream read protocol parameters")
	}

	// Now that we parsed the protocol parameters we can parse the commitment.
	api := s.API(commitmentBytes[0]) // fetch an api for the commitment version
	commitment, err := model.CommitmentFromBytes(commitmentBytes, api)
	if err != nil {
		return errors.Wrap(err, "failed to parse commitment")
	}

	if err := s.SetLatestCommitment(commitment); err != nil {
		return errors.Wrap(err, "failed to set latest commitment")
	}

	return nil
}

func (s *Settings) String() string {
	builder := stringify.NewStructBuilder("Settings")
	builder.AddField(stringify.NewStructField("IsSnapshotImported", s.IsSnapshotImported()))
	builder.AddField(stringify.NewStructField("LatestCommitment", s.LatestCommitment()))
	builder.AddField(stringify.NewStructField("LatestFinalizedSlot", s.LatestFinalizedSlot()))
	if err := s.store.Iterate([]byte{protocolParametersKey}, func(key kvstore.Key, value kvstore.Value) bool {
		params, _, err := iotago.ProtocolParametersFromBytes(value)
		if err != nil {
			panic(err)
		}
		builder.AddField(stringify.NewStructField(fmt.Sprintf("protocolParameters(%s)", key), params))

		return true
	}); err != nil {
		panic(err)
	}

	return builder.String()
}

type protocolVersionEpochStart struct {
	Version    iotago.Version
	StartEpoch iotago.EpochIndex
}
