package retainer

import (
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

type BlockMetadata struct {
	BlockID            iotago.BlockID
	BlockState         api.BlockState
	BlockFailureReason api.BlockFailureReason

	TransactionID            iotago.TransactionID
	TransactionState         api.TransactionState
	TransactionFailureReason api.TransactionFailureReason
}

func (m *BlockMetadata) BlockMetadataResponse() *api.BlockMetadataResponse {
	return &api.BlockMetadataResponse{
		BlockID:             m.BlockID,
		BlockState:          m.BlockState.String(),
		BlockFailureReason:  m.BlockFailureReason,
		TransactionMetadata: m.TransactionMetadataResponse(),
	}
}

func (m *BlockMetadata) TransactionMetadataResponse() *api.TransactionMetadataResponse {
	if m.TransactionState == api.TransactionStateNoTransaction {
		return nil
	}

	return &api.TransactionMetadataResponse{
		TransactionID:            m.TransactionID,
		TransactionState:         m.TransactionState.String(),
		TransactionFailureReason: m.TransactionFailureReason,
	}
}
