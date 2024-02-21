package retainer

import (
	"github.com/iotaledger/hive.go/runtime/module"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

// TODO Remove old general interface after TransactionRetainer and ValidatorsCache is done
// Retainer keeps and resolves all the information needed in the API and INX.
type Retainer interface {
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
	BlockMetadata(blockID iotago.BlockID) (*api.BlockMetadataResponse, error)

	// Reset resets the component to a clean state as if it was created at the last commitment.
	Reset()

	// Interface embeds the required methods of the module.Interface.
	module.Interface
}

// TransactionRetainer keeps and resolves all the information needed in the API and INX.
type TransactionRetainer interface {
	TransactionMetadata(txID iotago.TransactionID) (*api.TransactionMetadataResponse, error)
	RetainTransactionFailure(iotago.TransactionID, error)

	// Reset resets the component to a clean state as if it was created at the last commitment.
	Reset()

	module.Interface
}
