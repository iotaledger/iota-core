package coreapi

import (
	iotago "github.com/iotaledger/iota.go/v4"
)

func getCommitment(index iotago.SlotIndex) (*commitmentInfoResponse, error) {
	commitment, err := deps.Protocol.MainEngineInstance().Storage.Permanent.Commitments().Load(index)
	if err != nil {
		return nil, err
	}

	return &commitmentInfoResponse{
		Index:            commitment.Index(),
		PrevID:           commitment.PrevID().ToHex(),
		RootsID:          commitment.RootsID().ToHex(),
		CumulativeWeight: commitment.CumulativeWeight(),
	}, nil
}
