package retainer

import (
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/model"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/nodeclient/apimodels"
)

// Retainer keeps and resolves all the information needed in the API and INX.
type Retainer interface {
	Block(iotago.BlockID) (*model.Block, error)
	BlockMetadata(blockID iotago.BlockID) (*BlockMetadata, error)
	RetainBlockFailure(iotago.BlockID, apimodels.BlockFailureReason)
	RetainTransactionFailure(iotago.TransactionID, iotago.SlotIndex, apimodels.TransactionFailureReason)

	// Interface embeds the required methods of the module.Interface.
	module.Interface
}
