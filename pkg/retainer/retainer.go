package retainer

import (
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

// Retainer keeps and resolves all the information needed in the API and INX.
type Retainer interface {
	BlockMetadata(blockID iotago.BlockID) (*BlockMetadata, error)

	RegisteredValidatorsCache(uint32) ([]*api.ValidatorResponse, bool)
	RetainRegisteredValidatorsCache(uint32, []*api.ValidatorResponse)

	RetainBlockFailure(*blocks.Block, api.BlockFailureReason)
	RetainModelBlockFailure(*model.Block, api.BlockFailureReason)
	RetainTransactionFailure(iotago.TransactionID, error)

	// Reset resets the component to a clean state as if it was created at the last commitment.
	Reset()

	// Interface embeds the required methods of the module.Interface.
	module.Interface
}
