package model

import (
	"bytes"
	"time"

	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
)

type SignaledBlock struct {
	ID                      iotago.BlockID    `serix:"0"`
	IssuingTime             time.Time         `serix:"1"`
	HighestSupportedVersion iotago.Version    `serix:"2"`
	ProtocolParametersHash  iotago.Identifier `serix:"3"`
}

func NewSignaledBlock(blockID iotago.BlockID, block *iotago.Block, validationBlock *iotago.ValidationBlockBody) *SignaledBlock {
	return &SignaledBlock{
		ID:                      blockID,
		IssuingTime:             block.Header.IssuingTime,
		HighestSupportedVersion: validationBlock.HighestSupportedVersion,
		ProtocolParametersHash:  validationBlock.ProtocolParametersHash,
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

func (s *SignaledBlock) Bytes(apiForSlot iotago.API) ([]byte, error) {
	return apiForSlot.Encode(s)
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
