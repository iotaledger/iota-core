package prunable

import (
	"bytes"
	"time"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/inx-app/pkg/api"
	"github.com/iotaledger/iota-core/pkg/core/account"
	iotago "github.com/iotaledger/iota.go/v4"
)

type SignaledBlock struct {
	ID                      iotago.BlockID    `serix:"0"`
	IssuingTime             time.Time         `serix:"1"`
	HighestSupportedVersion iotago.Version    `serix:"2"`
	ProtocolParametersHash  iotago.Identifier `serix:"3"`
}

func NewSignaledBlock(blockID iotago.BlockID, block *iotago.ProtocolBlock, validationBlock *iotago.ValidationBlock) *SignaledBlock {
	return &SignaledBlock{
		ID:                      blockID,
		IssuingTime:             block.IssuingTime,
		HighestSupportedVersion: validationBlock.HighestSupportedVersion,
		ProtocolParametersHash:  validationBlock.ProtocolParametersHash,
	}
}

func SignaledBlockFromBytesFunc(decodeAPI iotago.API) func([]byte) (*SignaledBlock, int, error) {
	return func(bytes []byte) (*SignaledBlock, int, error) {
		signaledBlock := new(SignaledBlock)
		consumedBytes, err := decodeAPI.Decode(bytes, signaledBlock)
		if err != nil {
			return nil, 0, err
		}

		return signaledBlock, consumedBytes, nil
	}
}

func (s *SignaledBlock) Compare(other *SignaledBlock) int {
	switch {
	case s == nil && other == nil:
		return 0
	case s == nil:
		return -1
	case other == nil:
		return 1
	case s.HighestSupportedVersion > other.HighestSupportedVersion:
		return 1
	case s.HighestSupportedVersion < other.HighestSupportedVersion:
		return -1
	case s.IssuingTime.After(other.IssuingTime):
		return 1
	case s.IssuingTime.Before(other.IssuingTime):
		return -1
	default:
		return bytes.Compare(lo.PanicOnErr(s.ID.Bytes()), lo.PanicOnErr(other.ID.Bytes()))
	}
}

type UpgradeSignals struct {
	slot  iotago.SlotIndex
	store *kvstore.TypedStore[account.SeatIndex, *SignaledBlock]

	apiProvider api.Provider
}

func NewUpgradeSignals(slot iotago.SlotIndex, store kvstore.KVStore, apiProvider api.Provider) *UpgradeSignals {
	signaledBlockToBytes := func(signaledBlock *SignaledBlock) ([]byte, error) {
		return apiProvider.APIForSlot(slot).Encode(signaledBlock)
	}

	return &UpgradeSignals{
		slot:        slot,
		store:       kvstore.NewTypedStore(store, account.SeatIndex.Bytes, account.SeatIndexFromBytes, signaledBlockToBytes, SignaledBlockFromBytesFunc(apiProvider.APIForSlot(slot))),
		apiProvider: apiProvider,
	}
}

func (u *UpgradeSignals) Load(seat account.SeatIndex) (*SignaledBlock, error) {
	signaledBlock, err := u.store.Get(seat)
	if err != nil {
		if ierrors.Is(err, kvstore.ErrKeyNotFound) {
			//nolint:nilnil // expected behavior
			return nil, nil
		}

		return nil, ierrors.Wrapf(err, "failed to get signaled block for seat %d", seat)
	}

	return signaledBlock, nil
}

func (u *UpgradeSignals) Store(signaledBlockPerSeat map[account.SeatIndex]*SignaledBlock) error {
	for seat, signaledBlock := range signaledBlockPerSeat {
		if err := u.store.Set(seat, signaledBlock); err != nil {
			return ierrors.Wrapf(err, "failed to set signaled block for seat %d", seat)
		}
	}

	return nil
}

func (u *UpgradeSignals) Stream(consumer func(account.SeatIndex, *SignaledBlock) error) error {
	var innerErr error
	if storageErr := u.store.Iterate(kvstore.EmptyPrefix, func(seatIndex account.SeatIndex, signaledBlock *SignaledBlock) (advance bool) {
		innerErr = consumer(seatIndex, signaledBlock)
		return innerErr != nil
	}); storageErr != nil {
		return ierrors.Wrap(storageErr, "failed to iterate over upgrade signals")
	}

	return innerErr
}

func (u *UpgradeSignals) StreamBytes(consumer func([]byte, []byte) error) error {
	var innerErr error
	if storageErr := u.store.KVStore().Iterate(kvstore.EmptyPrefix, func(key kvstore.Key, value kvstore.Value) (advance bool) {
		innerErr = consumer(key, value)
		return innerErr == nil
	}); storageErr != nil {
		return ierrors.Wrap(storageErr, "failed to iterate over upgrade signals")
	}

	return innerErr
}
