package permanent

import (
	"encoding/binary"
	"fmt"
	"io"
	"sync"

	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/stringify"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/utils"
	iotago "github.com/iotaledger/iota.go/v4"
)

const (
	snapshotImportedKey = iota
	latestCommitmentKey
	latestFinalizedSlotKey
	protocolParametersKey
)

type APIBySlotIndexProviderFunc func(iotago.SlotIndex) iotago.API

type Settings struct {
	mutex sync.RWMutex
	store kvstore.KVStore

	apiByVersion map[byte]iotago.API
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

func (s *Settings) HighestSupportedProtocolVersion() byte {
	return iotago.LatestProtocolVersion()
}

func (s *Settings) LatestAPI() iotago.API {
	return s.API(iotago.LatestProtocolVersion())
}

func (s *Settings) APIForSlotIndex(_ iotago.SlotIndex) iotago.API {
	//TODO: map index to epoch to version
	return s.LatestAPI()
}

func (s *Settings) API(version byte) iotago.API {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	if api, exists := s.apiByVersion[version]; exists {
		return api
	}

	protocolParams := s.protocolParameters(version)

	if protocolParams == nil {
		panic(fmt.Errorf("protocol parameters for version %d not found", version))
	}

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

func (s *Settings) protocolParameters(version byte) iotago.ProtocolParameters {
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
	return lo.PanicOnErr(iotago.SlotIndexFromBytes(bytes))
}

func (s *Settings) SetLatestFinalizedSlot(index iotago.SlotIndex) (err error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	return s.store.Set([]byte{latestFinalizedSlotKey}, index.Bytes())
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

	if _, err = writer.Write(s.LatestFinalizedSlot().Bytes()); err != nil {
		return errors.Wrap(err, "failed to write latest finalized slot")
	}

	var relativeCountersPosition int64

	var paramsCount uint8
	var paramsByteCount uint32

	// The amount of protocolParameters contained within this snapshot.
	if err := utils.WriteValueFunc(writer, paramsCount, &relativeCountersPosition); err != nil {
		return errors.Wrap(err, "unable to write protocol parameters count")
	}

	// The amount of bytes the protocolParameters consume in this snapshot.
	if err := utils.WriteValueFunc(writer, paramsByteCount, &relativeCountersPosition); err != nil {
		return errors.Wrap(err, "unable to write protocol parameters count")
	}

	var innerErr error
	if err := s.store.Iterate([]byte{protocolParametersKey}, func(key kvstore.Key, value kvstore.Value) bool {
		if err := utils.WriteBytesFunc(writer, value, &relativeCountersPosition); err != nil {
			innerErr = err
			return false
		}
		paramsCount++
		paramsByteCount += uint32(len(value))

		return true
	}); err != nil {
		return errors.Wrap(err, "failed to iterate over protocol parameters")
	}
	if innerErr != nil {
		return errors.Wrap(innerErr, "failed to write protocol parameters")
	}

	// seek back to the file position of the counters
	if _, err := writer.Seek(-relativeCountersPosition, io.SeekCurrent); err != nil {
		return fmt.Errorf("unable to seek counter placeholders: %w", err)
	}

	var countersSize int64

	if err := utils.WriteValueFunc(writer, paramsCount, &countersSize); err != nil {
		return errors.Wrap(err, "unable to write protocol parameters count")
	}

	if err := utils.WriteValueFunc(writer, paramsByteCount, &countersSize); err != nil {
		return errors.Wrap(err, "unable to write protocol parameters byte count")
	}

	// seek back to the last write position
	if _, err := writer.Seek(relativeCountersPosition-countersSize, io.SeekCurrent); err != nil {
		return fmt.Errorf("unable to seek to last written position: %w", err)
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
	slotIndex, err := iotago.SlotIndexFromBytes(latestFinalizedSlotBytes[:])
	if err != nil {
		return errors.Wrap(err, "failed to parse latest finalized slot")
	}
	if err := s.SetLatestFinalizedSlot(slotIndex); err != nil {
		return errors.Wrap(err, "failed to set latest finalized slot")
	}

	var paramsCount uint8
	var paramsByteCount uint32

	if err = binary.Read(reader, binary.LittleEndian, &paramsCount); err != nil {
		return errors.Wrap(err, "failed to read protocol parameters count")
	}

	if err = binary.Read(reader, binary.LittleEndian, &paramsByteCount); err != nil {
		return errors.Wrap(err, "failed to read protocol parameters byte count")
	}

	parameterBytes := make([]byte, paramsByteCount)
	if err = binary.Read(reader, binary.LittleEndian, parameterBytes); err != nil {
		return errors.Wrap(err, "failed to read protocol parameters bytes")
	}

	var offset int
	for i := uint8(0); i < paramsCount; i++ {
		params, bytesRead, err := iotago.ProtocolParametersFromBytes(parameterBytes[offset:])
		if err != nil {
			return errors.Wrapf(err, "failed to parse protocol parameters at index %d", i)
		}
		offset += bytesRead

		if err := s.StoreProtocolParameters(params); err != nil {
			return errors.Wrapf(err, "failed to store protocol parameters at index %d", i)
		}
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

	return s.SetSnapshotImported()
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
