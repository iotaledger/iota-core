package mempool

import (
	iotago "github.com/iotaledger/iota.go/v4"
)

type MemPool interface {
	Events() *MemPoolEvents

	ProcessTransaction(tx Transaction) error

	SetTransactionInclusionSlot(id TransactionID, inclusionSlot iotago.SlotIndex) error

	EvictTransaction(id TransactionID) error

	ConflictDAG() interface{}
}
