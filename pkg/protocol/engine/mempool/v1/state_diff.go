package mempoolv1

import (
	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/ds/orderedmap"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore/mapdb"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	iotago "github.com/iotaledger/iota.go/v4"
)

type StateDiff struct {
	index iotago.SlotIndex

	spentOutputs *shrinkingmap.ShrinkingMap[iotago.OutputID, mempool.StateMetadata]

	createdOutputs *shrinkingmap.ShrinkingMap[iotago.OutputID, mempool.StateMetadata]

	executedTransactions *orderedmap.OrderedMap[iotago.TransactionID, mempool.TransactionMetadata]

	stateUsageCounters *shrinkingmap.ShrinkingMap[iotago.OutputID, int]

	mutations ads.Set[iotago.TransactionID]
}

func NewStateDiff(index iotago.SlotIndex) *StateDiff {
	return &StateDiff{
		index:                index,
		spentOutputs:         shrinkingmap.New[iotago.OutputID, mempool.StateMetadata](),
		createdOutputs:       shrinkingmap.New[iotago.OutputID, mempool.StateMetadata](),
		executedTransactions: orderedmap.New[iotago.TransactionID, mempool.TransactionMetadata](),
		stateUsageCounters:   shrinkingmap.New[iotago.OutputID, int](),
		mutations:            ads.NewSet(mapdb.NewMapDB(), iotago.Identifier.Bytes, iotago.IdentifierFromBytes),
	}
}

func (s *StateDiff) Index() iotago.SlotIndex {
	return s.index
}

func (s *StateDiff) DestroyedStates() *shrinkingmap.ShrinkingMap[iotago.OutputID, mempool.StateMetadata] {
	return s.spentOutputs
}

func (s *StateDiff) CreatedStates() *shrinkingmap.ShrinkingMap[iotago.OutputID, mempool.StateMetadata] {
	return s.createdOutputs
}

func (s *StateDiff) ExecutedTransactions() *orderedmap.OrderedMap[iotago.TransactionID, mempool.TransactionMetadata] {
	return s.executedTransactions
}

func (s *StateDiff) Mutations() ads.Set[iotago.TransactionID] {
	return s.mutations
}

func (s *StateDiff) updateCompactedStateChanges(transaction *TransactionMetadata, direction int) {
	transaction.Inputs().Range(func(input mempool.StateMetadata) {
		s.compactStateChanges(input, s.stateUsageCounters.Compute(input.ID(), func(currentValue int, _ bool) int {
			return currentValue - direction
		}))
	})

	transaction.Outputs().Range(func(output mempool.StateMetadata) {
		s.compactStateChanges(output, s.stateUsageCounters.Compute(output.ID(), func(currentValue int, _ bool) int {
			return currentValue + direction
		}))
	})
}

func (s *StateDiff) AddTransaction(transaction *TransactionMetadata) error {
	if _, exists := s.executedTransactions.Set(transaction.ID(), transaction); !exists {
		if err := s.mutations.Add(transaction.ID()); err != nil {
			return ierrors.Wrapf(err, "failed to add transaction %s to state diff", transaction.ID())
		}
		s.updateCompactedStateChanges(transaction, 1)

		transaction.OnPending(func() {
			s.RollbackTransaction(transaction)
		})
	}

	return nil
}

func (s *StateDiff) RollbackTransaction(transaction *TransactionMetadata) {
	if s.executedTransactions.Delete(transaction.ID()) {
		s.mutations.Delete(transaction.ID())
		s.updateCompactedStateChanges(transaction, -1)
	}
}

func (s *StateDiff) compactStateChanges(output mempool.StateMetadata, newValue int) {
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
