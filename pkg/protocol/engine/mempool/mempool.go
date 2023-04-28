package mempool

import (
	"iota-core/pkg/protocol/engine/ledger"
	"iota-core/pkg/protocol/engine/mempool/conflictdag"

	iotago "github.com/iotaledger/iota.go/v4"
)

type MemPool[VotePower conflictdag.VotePowerType[VotePower]] interface {
	Events() *Events

	ConflictDAG() conflictdag.ConflictDAG[iotago.TransactionID, iotago.OutputID, VotePower]

	AddTransaction(transaction Transaction) (metadata TransactionWithMetadata, err error)

	RemoveTransaction(id iotago.TransactionID)

	Transaction(id iotago.TransactionID) (metadata TransactionWithMetadata, exists bool)

	State(reference ledger.StateReference) (state StateWithMetadata, err error)

	SetTransactionIncluded(id iotago.TransactionID, inclusionSlot iotago.SlotIndex) error
}
