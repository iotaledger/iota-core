package retainer

import (
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/nodeclient/apimodels"
)

type BlockMetadata struct {
	BlockID                  iotago.BlockID
	BlockState               apimodels.BlockState
	BlockFailureReason       apimodels.BlockFailureReason
	TransactionState         apimodels.TransactionState
	TransactionFailureReason apimodels.TransactionFailureReason
}

func (b *BlockMetadata) BlockMetadataResponse() *apimodels.BlockMetadataResponse {
	response := &apimodels.BlockMetadataResponse{
		BlockID:                  b.BlockID,
		BlockState:               b.BlockState.String(),
		BlockFailureReason:       b.BlockFailureReason,
		TransactionFailureReason: b.TransactionFailureReason,
	}

	if b.TransactionState != apimodels.TransactionStateNoTransaction {
		response.TransactionState = b.TransactionState.String()
	}

	return response
}
