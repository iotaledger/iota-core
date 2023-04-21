package mempool

import (
	iotago2 "iota-core/pkg/iotago"

	iotago "github.com/iotaledger/iota.go/v4"
)

type MemPool interface {
	Events() *MemPoolEvents

	ProcessTransaction(tx iotago2.Transaction) error

	SetTransactionInclusionSlot(id iotago.TransactionID, inclusionSlot iotago.SlotIndex) error

	EvictTransaction(id iotago.TransactionID) error

	ConflictDAG() interface{}
}
