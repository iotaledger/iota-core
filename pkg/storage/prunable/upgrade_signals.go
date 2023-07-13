package prunable

import (
	"bytes"
	"time"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/core/api"
	iotago "github.com/iotaledger/iota.go/v4"
)

type SignalledBlock struct {
	ID                      iotago.BlockID `serix:"0"`
	IssuingTime             time.Time      `serix:"1"`
	HighestSupportedVersion iotago.Version `serix:"2"`
}

func NewSignalledBlock(blockID iotago.BlockID, block *iotago.ProtocolBlock) (*SignalledBlock, error) {
	validationBlock, isValidationBlock := block.Block.(*iotago.ValidationBlock)
	if !isValidationBlock {
		return nil, ierrors.New("block is not a validation block")
	}

	return &SignalledBlock{
		ID:                      blockID,
		IssuingTime:             block.IssuingTime,
		HighestSupportedVersion: validationBlock.HighestSupportedVersion,
	}, nil
}

func (s *SignalledBlock) Compare(other *SignalledBlock) int {
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
	store *kvstore.TypedStore[account.SeatIndex, *SignalledBlock]

	apiProvider api.Provider
}

func NewUpgradeSignals(slot iotago.SlotIndex, store kvstore.KVStore, apiProvider api.Provider) *UpgradeSignals {
	signalledBlockToBytes := func(signalledBlock *SignalledBlock) ([]byte, error) {
		return apiProvider.APIForSlot(slot).Encode(signalledBlock)
	}

	signalledBlockFromBytes := func(bytes []byte) (*SignalledBlock, int, error) {
		signalledBlock := new(SignalledBlock)
		consumedBytes, err := apiProvider.APIForSlot(slot).Decode(bytes, signalledBlock)
		if err != nil {
			return nil, 0, err
		}

		return signalledBlock, consumedBytes, nil
	}

	return &UpgradeSignals{
		slot:        slot,
		store:       kvstore.NewTypedStore(store, account.SeatIndex.Bytes, account.SeatIndexFromBytes, signalledBlockToBytes, signalledBlockFromBytes),
		apiProvider: apiProvider,
	}
}

func (p *UpgradeSignals) Load(seat account.SeatIndex) (*SignalledBlock, error) {
	signalledBlock, err := p.store.Get(seat)
	if err != nil {
		if ierrors.Is(err, kvstore.ErrKeyNotFound) {
			//nolint:nilnil // expected behavior
			return nil, nil
		}

		return nil, ierrors.Wrapf(err, "failed to get signaled block for seat %d", seat)
	}

	return signalledBlock, nil
}

func (p *UpgradeSignals) Store(signalledBlockPerSeat map[account.SeatIndex]*SignalledBlock) error {
	for seat, signalledBlock := range signalledBlockPerSeat {
		if err := p.store.Set(seat, signalledBlock); err != nil {
			return ierrors.Wrapf(err, "failed to set signaled block for seat %d", seat)
		}
	}

	return nil
}
