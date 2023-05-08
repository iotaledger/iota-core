package mempoolv1

import (
	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/ds/orderedmap"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	iotago "github.com/iotaledger/iota.go/v4"
)

type StateDiff struct {
	index iotago.SlotIndex

	spentOutputs   *shrinkingmap.ShrinkingMap[iotago.OutputID, mempool.StateWithMetadata]
	createdOutputs *shrinkingmap.ShrinkingMap[iotago.OutputID, mempool.StateWithMetadata]

	executedTransactions *orderedmap.OrderedMap[iotago.TransactionID, mempool.TransactionWithMetadata]

	mutations *ads.Set[iotago.TransactionID, *iotago.TransactionID]
}

func (s *StateDiff) Index() iotago.SlotIndex {
	return s.index
}

func (s *StateDiff) SpentOutputs() *shrinkingmap.ShrinkingMap[iotago.OutputID, mempool.StateWithMetadata] {
	return s.spentOutputs
}

func (s *StateDiff) CreatedOutputs() *shrinkingmap.ShrinkingMap[iotago.OutputID, mempool.StateWithMetadata] {
	return s.createdOutputs
}

func (s *StateDiff) ExecutedTransactions() *orderedmap.OrderedMap[iotago.TransactionID, mempool.TransactionWithMetadata] {
	return s.executedTransactions
}

func (s *StateDiff) Mutations() *ads.Set[iotago.TransactionID, *iotago.TransactionID] {
	return s.mutations
}

func (s *StateDiff) AddTransaction(transaction *TransactionWithMetadata) {
	// add to mutations
}

var _ mempool.StateDiff = new(StateDiff)
