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

	// Interface embeds the required methods of the module.Interface.
	module.Interface
}

// TransactionRetainer keeps and resolves all the tx related information needed in the API and INX.
type TransactionRetainer interface {
	// UpdateTransactionMetadata updates the metadata of a transaction.
	UpdateTransactionMetadata(txID iotago.TransactionID, validSignature bool, earliestAttachmentSlot iotago.SlotIndex, state api.TransactionState, txErr error)

	// TransactionMetadata returns the metadata of a transaction.
	TransactionMetadata(txID iotago.TransactionID) (*api.TransactionMetadataResponse, error)

	// Rollback rolls back the component state as if the last committed slot was targetSlot.
	Rollback(targetSlot iotago.SlotIndex) error

	// Reset resets the component to a clean state as if it was created at the last commitment.
	Reset()

	// Prune prunes the component state as if the last pruned slot was targetSlot.
	Prune(targetSlot iotago.SlotIndex) error

	module.Interface
}
