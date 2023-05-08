package mempool

import (
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledger"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/conflictdag"
	iotago "github.com/iotaledger/iota.go/v4"
)

type MemPool[VotePower conflictdag.VotePowerType[VotePower]] interface {
	AttachTransaction(transaction Transaction, blockID iotago.BlockID) (storedTransaction TransactionWithMetadata, err error)

	State(reference ledger.StateReference) (state StateWithMetadata, err error)

	Transaction(id iotago.TransactionID) (transaction TransactionWithMetadata, exists bool)

	TransactionByAttachment(blockID iotago.BlockID) (transaction TransactionWithMetadata, exists bool)

	MarkAttachmentOrphaned(blockID iotago.BlockID) bool

	MarkAttachmentIncluded(blockID iotago.BlockID) bool

	StateDiff(index iotago.SlotIndex) StateDiff

	Evict(slotIndex iotago.SlotIndex)

	ConflictDAG() conflictdag.ConflictDAG[iotago.TransactionID, iotago.OutputID, VotePower]

	Events() *Events
}
