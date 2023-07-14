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

type SignaledBlock struct {
	ID                      iotago.BlockID    `serix:"0"`
	IssuingTime             time.Time         `serix:"1"`
	HighestSupportedVersion iotago.Version    `serix:"2"`
	ProtocolParametersHash  iotago.Identifier `serix:"3"`
}

func NewSignaledBlock(blockID iotago.BlockID, block *iotago.ProtocolBlock) (*SignaledBlock, error) {
	validationBlock, isValidationBlock := block.Block.(*iotago.ValidationBlock)
	if !isValidationBlock {
		return nil, ierrors.New("block is not a validation block")
	}

	return &SignaledBlock{
		ID:                      blockID,
		IssuingTime:             block.IssuingTime,
		HighestSupportedVersion: validationBlock.HighestSupportedVersion,
		ProtocolParametersHash:  validationBlock.ProtocolParametersHash,
	}, nil
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

	signaledBlockFromBytes := func(bytes []byte) (*SignaledBlock, int, error) {
		signaledBlock := new(SignaledBlock)
		consumedBytes, err := apiProvider.APIForSlot(slot).Decode(bytes, signaledBlock)
		if err != nil {
			return nil, 0, err
		}

		return signaledBlock, consumedBytes, nil
	}

	return &UpgradeSignals{
		slot:        slot,
		store:       kvstore.NewTypedStore(store, account.SeatIndex.Bytes, account.SeatIndexFromBytes, signaledBlockToBytes, signaledBlockFromBytes),
		apiProvider: apiProvider,
	}
}

func (p *UpgradeSignals) Load(seat account.SeatIndex) (*SignaledBlock, error) {
	signaledBlock, err := p.store.Get(seat)
	if err != nil {
		if ierrors.Is(err, kvstore.ErrKeyNotFound) {
			//nolint:nilnil // expected behavior
			return nil, nil
		}

		return nil, ierrors.Wrapf(err, "failed to get signaled block for seat %d", seat)
	}

	return signaledBlock, nil
}

func (p *UpgradeSignals) Store(signaledBlockPerSeat map[account.SeatIndex]*SignaledBlock) error {
	for seat, signaledBlock := range signaledBlockPerSeat {
		if err := p.store.Set(seat, signaledBlock); err != nil {
			return ierrors.Wrapf(err, "failed to set signaled block for seat %d", seat)
		}
	}

	return nil
}
