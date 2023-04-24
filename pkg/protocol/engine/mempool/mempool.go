package mempool

import (
	iotago "github.com/iotaledger/iota.go/v4"
)

type MemPool interface {
	Events() *Events

	ProcessTransaction(transaction Transaction) error

	TransactionMetadata(id iotago.TransactionID) (metadata TransactionWithMetadata, exists bool)

	SetTransactionInclusionSlot(id iotago.TransactionID, inclusionSlot iotago.SlotIndex) error

	EvictTransaction(id iotago.TransactionID) error

	ConflictDAG() interface{}
}
