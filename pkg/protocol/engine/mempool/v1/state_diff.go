package mempoolv1

import (
	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/ds/orderedmap"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/kvstore/mapdb"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	iotago "github.com/iotaledger/iota.go/v4"
)

type StateDiff struct {
	index iotago.SlotIndex

	spentOutputs   *shrinkingmap.ShrinkingMap[iotago.OutputID, mempool.StateWithMetadata]
	createdOutputs *shrinkingmap.ShrinkingMap[iotago.OutputID, mempool.StateWithMetadata]

	executedTransactions *orderedmap.OrderedMap[iotago.TransactionID, mempool.TransactionWithMetadata]

	stateUsageCounters *shrinkingmap.ShrinkingMap[iotago.OutputID, int]

	mutations *ads.Set[iotago.TransactionID, *iotago.TransactionID]
}

func NewStateDiff(index iotago.SlotIndex) *StateDiff {
	return &StateDiff{index: index,
		spentOutputs:         shrinkingmap.New[iotago.OutputID, mempool.StateWithMetadata](),
		createdOutputs:       shrinkingmap.New[iotago.OutputID, mempool.StateWithMetadata](),
		executedTransactions: orderedmap.New[iotago.TransactionID, mempool.TransactionWithMetadata](),
		stateUsageCounters:   shrinkingmap.New[iotago.OutputID, int](),
		mutations:            ads.NewSet[iotago.TransactionID, *iotago.TransactionID](mapdb.NewMapDB()),
	}
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

func (s *StateDiff) updateCompactedStateChanges(transaction *TransactionWithMetadata, direction int) {
	transaction.Inputs().Range(func(input mempool.StateWithMetadata) {
		s.compactStateChanges(input, s.stateUsageCounters.Compute(input.ID(), func(currentValue int, _ bool) int {
			return currentValue - direction
		}))
	})

	transaction.Outputs().Range(func(output mempool.StateWithMetadata) {
		s.compactStateChanges(output, s.stateUsageCounters.Compute(output.ID(), func(currentValue int, _ bool) int {
			return currentValue + direction
		}))
	})
}

func (s *StateDiff) AddTransaction(transaction *TransactionWithMetadata) {
	s.executedTransactions.Set(transaction.ID(), transaction)
	s.mutations.Add(transaction.ID())
	s.updateCompactedStateChanges(transaction, 1)
}

func (s *StateDiff) RollbackTransaction(transaction *TransactionWithMetadata) {
	s.executedTransactions.Delete(transaction.ID())
	s.mutations.Delete(transaction.ID())
	s.updateCompactedStateChanges(transaction, -1)
}

func (s *StateDiff) compactStateChanges(output mempool.StateWithMetadata, newValue int) {
	switch {
	case newValue > 0:
		s.createdOutputs.Set(output.ID(), output)
	case newValue < 0:
		s.spentOutputs.Set(output.ID(), output)
	default:
		s.createdOutputs.Delete(output.ID())
		s.spentOutputs.Delete(output.ID())
	}
}

var _ mempool.StateDiff = new(StateDiff)
