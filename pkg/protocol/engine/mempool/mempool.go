package mempool

import (
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/conflictdag"
	iotago "github.com/iotaledger/iota.go/v4"
)

type MemPool[VotePower conflictdag.VotePowerType[VotePower]] interface {
	AttachTransaction(transaction Transaction, blockID iotago.BlockID) (storedTransaction TransactionMetadata, err error)

	OnTransactionAttached(callback func(metadata TransactionMetadata), opts ...event.Option)

	MarkAttachmentOrphaned(blockID iotago.BlockID) bool

	MarkAttachmentIncluded(block *blocks.Block) bool

	StateMetadata(reference iotago.IndexedUTXOReferencer) (state StateMetadata, err error)

	TransactionMetadata(id iotago.TransactionID) (transaction TransactionMetadata, exists bool)

	TransactionMetadataByAttachment(blockID iotago.BlockID) (transaction TransactionMetadata, exists bool)

	StateDiff(index iotago.SlotIndex) StateDiff

	Evict(slotIndex iotago.SlotIndex)
}
