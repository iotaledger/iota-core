package retainer

import (
	"github.com/iotaledger/hive.go/runtime/module"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/nodeclient/apimodels"
)

// Retainer keeps and resolves all the information needed in the API and INX.
type Retainer interface {
	BlockMetadata(blockID iotago.BlockID) (*BlockMetadata, error)

	RetainBlockFailure(iotago.BlockID, apimodels.BlockFailureReason)
	RetainTransactionFailure(iotago.BlockID, apimodels.TransactionFailureReason)

	// Interface embeds the required methods of the module.Interface.
	module.Interface
}
