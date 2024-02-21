package retainer

import (
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/model"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

// TODO Remove old general interface after merging Andrews PR and connect new block retainer
// Retainer keeps and resolves all the information needed in the API and INX.
type Retainer interface {
	BlockMetadata(blockID iotago.BlockID) (*BlockMetadata, error)

	RegisteredValidatorsCache(uint32) ([]*api.ValidatorResponse, bool)
	RetainRegisteredValidatorsCache(uint32, []*api.ValidatorResponse)

	RetainTransactionFailure(iotago.TransactionID, error)

	// Reset resets the component to a clean state as if it was created at the last commitment.
	Reset()

	// Interface embeds the required methods of the module.Interface.
	module.Interface
}

// BlockRetainer keeps and resolves all the information needed in the API and INX.
type BlockRetainer interface {
	BlockMetadata(blockID iotago.BlockID) (*BlockMetadata, error)
	RetainBlockFailure(*model.Block, api.BlockFailureReason)

	// Interface embeds the required methods of the module.Interface.
	module.Interface
}

// TransactionRetainer keeps and resolves all the information needed in the API and INX.
type TransactionRetainer interface {
	TransactionMetadata(txID iotago.TransactionID) (*TransactionMetadata, error)
	RetainTransactionFailure(iotago.TransactionID, error)

	module.Interface
}
