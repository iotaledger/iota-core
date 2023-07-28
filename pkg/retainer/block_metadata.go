package retainer

import (
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/nodeclient/apimodels"
)

type BlockMetadata struct {
	BlockID            iotago.BlockID
	BlockState         apimodels.BlockState
	BlockFailureReason apimodels.BlockFailureReason
	TxState            apimodels.TransactionState
	TxFailureReason    apimodels.TransactionFailureReason
}

func (b *BlockMetadata) BlockMetadataResponse() *apimodels.BlockMetadataResponse {
	response := &apimodels.BlockMetadataResponse{
		BlockID:            b.BlockID.ToHex(),
		BlockState:         b.BlockState.String(),
		BlockFailureReason: b.BlockFailureReason,
		TxFailureReason:    b.TxFailureReason,
	}

	if b.TxState != apimodels.TransactionStateNoTransaction {
		response.TxState = b.TxState.String()
	}

	return response
}
