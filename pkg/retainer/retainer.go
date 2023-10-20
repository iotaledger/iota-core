package retainer

import (
	"github.com/iotaledger/hive.go/runtime/module"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/nodeclient/apimodels"
)

// Retainer keeps and resolves all the information needed in the API and INX.
type Retainer interface {
	BlockMetadata(blockID iotago.BlockID) (*BlockMetadata, error)

	RegisteredValidatorsCache(uint32) ([]*apimodels.ValidatorResponse, bool)
	RetainRegisteredValidatorsCache(uint32, []*apimodels.ValidatorResponse)

	RetainBlockFailure(iotago.BlockID, apimodels.BlockFailureReason)
	RetainTransactionFailure(iotago.BlockID, error)

	// Reset resets the component to a clean state as if it was created at the last commitment.
	Reset()

	// Interface embeds the required methods of the module.Interface.
	module.Interface
}
