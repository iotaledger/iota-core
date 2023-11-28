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

func (b *BlockMetadata) BlockMetadataResponse() *api.BlockMetadataResponse {
	return &api.BlockMetadataResponse{
		BlockID:             b.BlockID,
		BlockState:          b.BlockState.String(),
		BlockFailureReason:  b.BlockFailureReason,
		TransactionMetadata: b.TransactionMetadataResponse(),
	}
}

func (b *BlockMetadata) TransactionMetadataResponse() *api.TransactionMetadataResponse {
	if b.TransactionState == api.TransactionStateNoTransaction {
		return nil
	}

	return &api.TransactionMetadataResponse{
		TransactionID:            b.TransactionID,
		TransactionState:         b.TransactionState.String(),
		TransactionFailureReason: b.TransactionFailureReason,
	}
}
