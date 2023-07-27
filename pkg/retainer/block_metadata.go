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
		BlockID:    b.BlockID.ToHex(),
		BlockState: b.BlockState.String(),
	}

	if b.BlockFailureReason != apimodels.BlockFailureNone {
		response.BlockFailureReason = b.BlockFailureReason
	}

	if b.TxState != apimodels.TransactionStateUnknown {
		response.TxState = b.TxState.String()
	}

	if b.TxFailureReason != apimodels.NoTransactionFailureReason {
		response.TxFailureReason = b.TxFailureReason
	}

	return response
}
