package mempoolv1

import (
	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/ds/orderedmap"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	iotago "github.com/iotaledger/iota.go/v4"
)

type StateDiff struct {
	slot iotago.SlotIndex

	spentOutputs *shrinkingmap.ShrinkingMap[mempool.StateID, mempool.StateMetadata]

	createdOutputs *shrinkingmap.ShrinkingMap[mempool.StateID, mempool.StateMetadata]

	executedTransactions *orderedmap.OrderedMap[iotago.TransactionID, mempool.TransactionMetadata]

	stateUsageCounters *shrinkingmap.ShrinkingMap[mempool.StateID, int]

	mutations   ads.Set[iotago.Identifier, iotago.TransactionID]
	mutationsKV kvstore.KVStore
}

func NewStateDiff(slot iotago.SlotIndex, kv kvstore.KVStore) *StateDiff {
	return &StateDiff{
		slot:                 slot,
		spentOutputs:         shrinkingmap.New[mempool.StateID, mempool.StateMetadata](),
		createdOutputs:       shrinkingmap.New[mempool.StateID, mempool.StateMetadata](),
		executedTransactions: orderedmap.New[iotago.TransactionID, mempool.TransactionMetadata](),
		stateUsageCounters:   shrinkingmap.New[mempool.StateID, int](),
		mutations: ads.NewSet[iotago.Identifier](
			kv,
			iotago.Identifier.Bytes,
			iotago.IdentifierFromBytes,
			iotago.TransactionID.Bytes,
			iotago.TransactionIDFromBytes,
		),
		mutationsKV: kv,
	}
}

func (s *StateDiff) Slot() iotago.SlotIndex {
	return s.slot
}

func (s *StateDiff) DestroyedStates() *shrinkingmap.ShrinkingMap[mempool.StateID, mempool.StateMetadata] {
	return s.spentOutputs
}

func (s *StateDiff) CreatedStates() *shrinkingmap.ShrinkingMap[mempool.StateID, mempool.StateMetadata] {
	return s.createdOutputs
}

func (s *StateDiff) ExecutedTransactions() *orderedmap.OrderedMap[iotago.TransactionID, mempool.TransactionMetadata] {
	return s.executedTransactions
}

func (s *StateDiff) Mutations() ads.Set[iotago.Identifier, iotago.TransactionID] {
	return s.mutations
}

func (s *StateDiff) updateCompactedStateChanges(transaction *TransactionMetadata, direction int) {
	for _, input := range transaction.inputs {
		s.compactStateChanges(input, s.stateUsageCounters.Compute(input.state.StateID(), func(currentValue int, _ bool) int {
			return currentValue - direction
		}))
	}

	for _, output := range transaction.outputs {
		s.compactStateChanges(output, s.stateUsageCounters.Compute(output.state.StateID(), func(currentValue int, _ bool) int {
			return currentValue + direction
		}))
	}
}

func (s *StateDiff) AddTransaction(transaction *TransactionMetadata) error {
	if _, exists := s.executedTransactions.Set(transaction.ID(), transaction); !exists {
		if err := s.mutations.Add(transaction.ID()); err != nil {
			return ierrors.Wrapf(err, "failed to add transaction to state diff, txID: %s", transaction.ID())
		}
		s.updateCompactedStateChanges(transaction, 1)
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

// Reset resets the component to a clean state as if it was created at the last commitment.
func (s *StateDiff) Reset() error {
	s.spentOutputs = shrinkingmap.New[mempool.StateID, mempool.StateMetadata]()
	s.createdOutputs = shrinkingmap.New[mempool.StateID, mempool.StateMetadata]()
	s.executedTransactions = orderedmap.New[iotago.TransactionID, mempool.TransactionMetadata]()
	s.stateUsageCounters = shrinkingmap.New[mempool.StateID, int]()

	if err := s.mutationsKV.Clear(); err != nil {
		return ierrors.Wrap(err, "failed to clear mutations")
	}

	s.mutations = ads.NewSet[iotago.Identifier](
		s.mutationsKV,
		iotago.Identifier.Bytes,
		iotago.IdentifierFromBytes,
		iotago.TransactionID.Bytes,
		iotago.TransactionIDFromBytes,
	)

	return nil
}

func (s *StateDiff) compactStateChanges(stateMetadata *StateMetadata, usageCounter int) {
	switch {
	case usageCounter > 0:
		s.createdOutputs.Set(stateMetadata.state.StateID(), stateMetadata)
	case usageCounter < 0:
		if !stateMetadata.state.IsReadOnly() {
			s.spentOutputs.Set(stateMetadata.state.StateID(), stateMetadata)
		}
	default:
		s.createdOutputs.Delete(stateMetadata.state.StateID())
		s.spentOutputs.Delete(stateMetadata.state.StateID())
	}
}

var _ mempool.StateDiff = new(StateDiff)
