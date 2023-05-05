package mempool

import (
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledger"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/conflictdag"
	iotago "github.com/iotaledger/iota.go/v4"
)

type MemPool[VotePower conflictdag.VotePowerType[VotePower]] interface {
	Events() *Events

	ConflictDAG() conflictdag.ConflictDAG[iotago.TransactionID, iotago.OutputID, VotePower]

	AttachTransaction(transaction Transaction, blockID iotago.BlockID) (metadata TransactionWithMetadata, err error)

	RemoveTransaction(id iotago.TransactionID)

	Transaction(id iotago.TransactionID) (metadata TransactionWithMetadata, exists bool)

	State(reference ledger.StateReference) (state StateWithMetadata, err error)

	MarkAttachmentIncluded(blockID iotago.BlockID) error

	Evict(slotIndex iotago.SlotIndex)

	StateDiff(index iotago.SlotIndex) (*StateDiff, error)
}
