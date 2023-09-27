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
	slot iotago.SlotIndex

	spentOutputs *shrinkingmap.ShrinkingMap[iotago.OutputID, mempool.OutputStateMetadata]

	createdOutputs *shrinkingmap.ShrinkingMap[iotago.OutputID, mempool.OutputStateMetadata]

	executedTransactions *orderedmap.OrderedMap[iotago.TransactionID, mempool.TransactionMetadata]

	stateUsageCounters *shrinkingmap.ShrinkingMap[iotago.OutputID, int]

	mutations ads.Set[iotago.TransactionID]
}

func NewStateDiff(slot iotago.SlotIndex) *StateDiff {
	return &StateDiff{
		slot:                 slot,
		spentOutputs:         shrinkingmap.New[iotago.OutputID, mempool.OutputStateMetadata](),
		createdOutputs:       shrinkingmap.New[iotago.OutputID, mempool.OutputStateMetadata](),
		executedTransactions: orderedmap.New[iotago.TransactionID, mempool.TransactionMetadata](),
		stateUsageCounters:   shrinkingmap.New[iotago.OutputID, int](),
		mutations:            ads.NewSet(mapdb.NewMapDB(), iotago.TransactionID.Bytes, iotago.SlotIdentifierFromBytes),
	}
}

func (s *StateDiff) Slot() iotago.SlotIndex {
	return s.slot
}

func (s *StateDiff) DestroyedStates() *shrinkingmap.ShrinkingMap[iotago.OutputID, mempool.OutputStateMetadata] {
	return s.spentOutputs
}

func (s *StateDiff) CreatedStates() *shrinkingmap.ShrinkingMap[iotago.OutputID, mempool.OutputStateMetadata] {
	return s.createdOutputs
}

func (s *StateDiff) ExecutedTransactions() *orderedmap.OrderedMap[iotago.TransactionID, mempool.TransactionMetadata] {
	return s.executedTransactions
}

func (s *StateDiff) Mutations() ads.Set[iotago.TransactionID] {
	return s.mutations
}

func (s *StateDiff) updateCompactedStateChanges(transaction *TransactionMetadata, direction int) {
	transaction.Inputs().Range(func(input mempool.OutputStateMetadata) {
		s.compactStateChanges(input, s.stateUsageCounters.Compute(input.OutputID(), func(currentValue int, _ bool) int {
			return currentValue - direction
		}))
	})

	transaction.Outputs().Range(func(output mempool.OutputStateMetadata) {
		s.compactStateChanges(output, s.stateUsageCounters.Compute(output.OutputID(), func(currentValue int, _ bool) int {
			return currentValue + direction
		}))
	})
}

func (s *StateDiff) AddTransaction(transaction *TransactionMetadata, errorHandler func(error)) error {
	if _, exists := s.executedTransactions.Set(transaction.ID(), transaction); !exists {
		if err := s.mutations.Add(transaction.ID()); err != nil {
			return ierrors.Wrapf(err, "failed to add transaction to state diff, txID: %s", transaction.ID())
		}
		s.updateCompactedStateChanges(transaction, 1)

		transaction.OnPending(func() {
			if err := s.RollbackTransaction(transaction); err != nil {
				errorHandler(ierrors.Wrapf(err, "failed to rollback transaction, txID: %s", transaction.ID()))
			}
		})
	}

	return nil
}

func (s *StateDiff) RollbackTransaction(transaction *TransactionMetadata) error {
	if s.executedTransactions.Delete(transaction.ID()) {
		if _, err := s.mutations.Delete(transaction.ID()); err != nil {
			return ierrors.Wrapf(err, "failed to delete transaction from state diff's mutations, txID: %s", transaction.ID())
		}
		s.updateCompactedStateChanges(transaction, -1)
	}

	return nil
}

func (s *StateDiff) compactStateChanges(output mempool.OutputStateMetadata, newValue int) {
	switch {
	case newValue > 0:
		s.createdOutputs.Set(output.OutputID(), output)
	case newValue < 0:
		s.spentOutputs.Set(output.OutputID(), output)
	default:
		s.createdOutputs.Delete(output.OutputID())
		s.spentOutputs.Delete(output.OutputID())
	}
}

var _ mempool.StateDiff = new(StateDiff)
