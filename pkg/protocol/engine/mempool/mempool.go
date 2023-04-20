package mempool

import (
	"context"

	iotago "github.com/iotaledger/iota.go/v4"
)

type MemPool interface {
	// Events is a dictionary for MemPool related events.
	Events() *Events

	ProcessTransaction(tx Transaction, ctx context.Context) error

	SetTransactionInclusionSlot(id iotago.TransactionID, inclusionSlot iotago.SlotIndex) error

	EvictTransaction(id iotago.TransactionID) error

	ConflictDAG() interface{}
}
