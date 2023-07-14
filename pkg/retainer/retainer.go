package retainer

import (
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/model"
	iotago "github.com/iotaledger/iota.go/v4"
)

type RetainerStatus struct {
	TanglePruningIndex     iotago.SlotIndex
	LedgerPruningIndex     iotago.SlotIndex
	CommitmentPruningIndex iotago.SlotIndex
}

// Retainer keeps and resolves all the information needed in the API and INX.
type Retainer interface {
	Block(iotago.BlockID) (*model.Block, error)
	BlockMetadata(blockID iotago.BlockID) (*BlockMetadata, error)

	// Interface embeds the required methods of the module.Interface.
	module.Interface
}
