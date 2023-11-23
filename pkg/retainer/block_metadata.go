package retainer

import (
	"github.com/iotaledger/hive.go/ierrors"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

type BlockMetadata struct {
	BlockID                  iotago.BlockID
	BlockState               api.BlockState
	BlockFailureReason       api.BlockFailureReason
	TransactionState         api.TransactionState
	TransactionFailureReason api.TransactionFailureReason
}

func (b *BlockMetadata) BlockMetadataResponse() *api.BlockMetadataResponse {
	response := &api.BlockMetadataResponse{
		BlockID:                  b.BlockID,
		BlockState:               b.BlockState.String(),
		BlockFailureReason:       b.BlockFailureReason,
		TransactionFailureReason: b.TransactionFailureReason,
	}

	if b.TransactionState != api.TransactionStateNoTransaction {
		response.TransactionState = b.TransactionState.String()
	}

	return response
}

func (b *BlockMetadata) TransactionMetadataResponse() (*api.TransactionMetadataResponse, error) {
	if b.TransactionState == api.TransactionStateNoTransaction {
		return nil, ierrors.Errorf("Transaction not found")
	}

	response := &api.TransactionMetadataResponse{
		TransactionState:         b.TransactionState.String(),
		TransactionFailureReason: b.TransactionFailureReason,
	}

	return response, nil
}
