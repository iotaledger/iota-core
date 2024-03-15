package retainer

import (
	"github.com/iotaledger/hive.go/runtime/module"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

// BlockRetainer keeps and resolves all the block related information needed in the API and INX.
type BlockRetainer interface {
	BlockMetadata(blockID iotago.BlockID) (*api.BlockMetadataResponse, error)

	// Reset resets the component to a clean state as if it was created at the last commitment.
	Reset()

	// Interface embeds the required methods of the module.Module.
	module.Module
}

// TransactionRetainer keeps and resolves all the transaction-related metadata needed in the API and INX.
type TransactionRetainer interface {
	// TransactionMetadata returns the metadata of a transaction.
	TransactionMetadata(txID iotago.TransactionID) (*api.TransactionMetadataResponse, error)

	// Reset resets the component to a clean state as if it was created at the last commitment.
	Reset(targetSlot iotago.SlotIndex)

	module.Module
}
