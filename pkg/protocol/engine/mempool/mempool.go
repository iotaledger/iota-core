package mempool

import (
	"iota-core/pkg/protocol/engine/ledger"

	iotago "github.com/iotaledger/iota.go/v4"
)

type MemPool interface {
	Events() *Events

	ConflictDAG() interface{}

	AddTransaction(transaction Transaction) (metadata TransactionWithMetadata, err error)

	RemoveTransaction(id iotago.TransactionID)

	Transaction(id iotago.TransactionID) (metadata TransactionWithMetadata, exists bool)

	State(reference ledger.StateReference) (state StateWithMetadata, err error)

	SetTransactionIncluded(id iotago.TransactionID, inclusionSlot iotago.SlotIndex) error
}
