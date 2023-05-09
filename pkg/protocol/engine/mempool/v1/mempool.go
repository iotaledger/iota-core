package mempoolv1

import (
	"context"
	"errors"
	"sync"

	"golang.org/x/xerrors"

	"github.com/iotaledger/hive.go/core/memstorage"
	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/core/acceptance"
	"github.com/iotaledger/iota-core/pkg/core/promise"
	"github.com/iotaledger/iota-core/pkg/core/vote"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledger"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/conflictdag"
	mockedconflictdag "github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/conflictdag/mocked"
	iotago "github.com/iotaledger/iota.go/v4"
)

type MemPool[VotePower conflictdag.VotePowerType[VotePower]] struct {
	events *mempool.Events

	executeStateTransition mempool.VM

	requestInput ledger.StateReferenceResolver

	attachments *memstorage.IndexedStorage[iotago.SlotIndex, iotago.BlockID, *TransactionWithMetadata]

	cachedTransactions *shrinkingmap.ShrinkingMap[iotago.TransactionID, *TransactionWithMetadata]

	cachedStateRequests *shrinkingmap.ShrinkingMap[iotago.OutputID, *promise.Promise[*StateWithMetadata]]

	stateDiffs *shrinkingmap.ShrinkingMap[iotago.SlotIndex, *StateDiff]

	conflictDAG conflictdag.ConflictDAG[iotago.TransactionID, iotago.OutputID, VotePower]

	executionWorkers *workerpool.WorkerPool

	lastEvictedSlot iotago.SlotIndex

	evictionMutex sync.RWMutex
}

func New[VotePower conflictdag.VotePowerType[VotePower]](vm mempool.VM, inputResolver ledger.StateReferenceResolver, workers *workerpool.Group) *MemPool[VotePower] {
	m := &MemPool[VotePower]{
		events:                 mempool.NewEvents(),
		executeStateTransition: vm,
		requestInput:           inputResolver,
		attachments:            memstorage.NewIndexedStorage[iotago.SlotIndex, iotago.BlockID, *TransactionWithMetadata](),
		cachedTransactions:     shrinkingmap.New[iotago.TransactionID, *TransactionWithMetadata](),
		cachedStateRequests:    shrinkingmap.New[iotago.OutputID, *promise.Promise[*StateWithMetadata]](),
		stateDiffs:             shrinkingmap.New[iotago.SlotIndex, *StateDiff](),
		executionWorkers:       workers.CreatePool("executionWorkers", 1),
		conflictDAG:            mockedconflictdag.New[iotago.TransactionID, iotago.OutputID, VotePower](),
	}

	m.ConflictDAG().Events().ConflictAccepted.Hook(func(id iotago.TransactionID) {
		if transaction, exists := m.cachedTransactions.Get(id); !exists {
			m.updateAcceptance(transaction)
		}
	})

	return m
}

func (m *MemPool[VotePower]) AttachTransaction(transaction mempool.Transaction, blockID iotago.BlockID) (metadata mempool.TransactionWithMetadata, err error) {
	storedTransaction, isNew, err := m.storeTransaction(transaction, blockID)
	if err != nil {
		return nil, xerrors.Errorf("failed to store transaction: %w", err)
	}

	if isNew {
		m.setupTransactionLifecycle(storedTransaction)
	}

	return storedTransaction, nil
}

func (m *MemPool[VotePower]) MarkAttachmentOrphaned(blockID iotago.BlockID) bool {
	return m.updateAttachment(blockID, (*TransactionWithMetadata).MarkOrphaned)
}

func (m *MemPool[VotePower]) MarkAttachmentIncluded(blockID iotago.BlockID) bool {
	return m.updateAttachment(blockID, (*TransactionWithMetadata).MarkIncluded)
}

func (m *MemPool[VotePower]) Transaction(id iotago.TransactionID) (transaction mempool.TransactionWithMetadata, exists bool) {
	return m.cachedTransactions.Get(id)
}

func (m *MemPool[VotePower]) State(stateReference ledger.StateReference) (state mempool.StateWithMetadata, err error) {
	stateRequest, exists := m.cachedStateRequests.Get(stateReference.StateID())
	if !exists || !stateRequest.WasCompleted() {
		stateRequest = m.requestStateWithMetadata(stateReference)
	}

	stateRequest.OnSuccess(func(loadedState *StateWithMetadata) { state = loadedState })
	stateRequest.OnError(func(requestErr error) { err = xerrors.Errorf("failed to request state: %w", requestErr) })
	stateRequest.WaitComplete()

	return state, err
}

func (m *MemPool[VotePower]) TransactionByAttachment(blockID iotago.BlockID) (mempool.TransactionWithMetadata, bool) {
	return m.transactionByAttachment(blockID)
}

func (m *MemPool[VotePower]) StateDiff(index iotago.SlotIndex) mempool.StateDiff {
	if stateDiff, exists := m.stateDiffs.Get(index); exists {
		return stateDiff
	}

	return NewStateDiff(index)
}

func (m *MemPool[VotePower]) Evict(slotIndex iotago.SlotIndex) {
	if evictedAttachments := func() *shrinkingmap.ShrinkingMap[iotago.BlockID, *TransactionWithMetadata] {
		m.evictionMutex.Lock()
		defer m.evictionMutex.Unlock()

		m.lastEvictedSlot = slotIndex

		return m.attachments.Evict(slotIndex)
	}(); evictedAttachments != nil {
		evictedAttachments.ForEach(func(blockID iotago.BlockID, transaction *TransactionWithMetadata) bool {
			transaction.attachments.Evict(blockID)

			return true
		})
	}
}

func (m *MemPool[VotePower]) ConflictDAG() conflictdag.ConflictDAG[iotago.TransactionID, iotago.OutputID, VotePower] {
	return m.conflictDAG
}

func (m *MemPool[VotePower]) Events() *mempool.Events {
	return m.events
}

func (m *MemPool[VotePower]) setupTransactionLifecycle(transaction *TransactionWithMetadata) {
	transaction.lifecycle.OnStored(func() {
		m.events.TransactionStored.Trigger(transaction)

		m.solidifyInputs(transaction)
	})

	transaction.lifecycle.OnSolid(func() {
		m.events.TransactionSolid.Trigger(transaction)

		m.executeTransaction(transaction)
	})

	transaction.lifecycle.OnExecuted(func() {
		m.events.TransactionExecuted.Trigger(transaction)

		m.bookTransaction(transaction)
	})

	transaction.lifecycle.OnBooked(func() {
		m.events.TransactionBooked.Trigger(transaction)

		m.publishOutputs(transaction)
	})

	transaction.lifecycle.OnInvalid(func(err error) {
		m.events.TransactionInvalid.Trigger(transaction, err)
	})

	transaction.inclusion.OnAccepted(func() {
		m.events.TransactionAccepted.Trigger(transaction)

		if slotIndex := transaction.Attachments().EarliestIncludedSlot(); slotIndex > 0 {
			lo.Return1(m.stateDiffs.GetOrCreate(slotIndex, func() *StateDiff { return NewStateDiff(slotIndex) })).AddTransaction(transaction)
		}
	})

	transaction.Attachments().OnEarliestIncludedSlotUpdated(func(prevIndex, newIndex iotago.SlotIndex) {
		if transaction.inclusion.IsAccepted() {
			m.updateStateDiffs(transaction, prevIndex, newIndex)
		} else if newIndex > 0 {
			m.updateAcceptance(transaction)
		}
	})

	transaction.OnAllInputsAccepted(func() {
		m.updateAcceptance(transaction)
	})

	transaction.inclusion.OnCommitted(func() {
		m.cachedTransactions.Delete(transaction.ID())
	})

	transaction.inclusion.OnEvicted(func() {
		m.cachedTransactions.Delete(transaction.ID())
	})
}

func (m *MemPool[VotePower]) setupStateLifecycle(state *StateWithMetadata) {
	deleteIfNoConsumers := func() {
		if !m.cachedStateRequests.Delete(state.ID(), state.spentState.HasNoSpenders) && m.cachedStateRequests.Has(state.ID()) {
			state.spentState.OnAllSpendersRemoved(func() {
				m.cachedStateRequests.Delete(state.ID(), state.spentState.HasNoSpenders)
			})
		}
	}

	state.inclusionState.OnCommitted(func() {
		deleteIfNoConsumers()
	})

	state.inclusionState.OnEvicted(func() {
		m.cachedStateRequests.Delete(state.ID())
	})
}

func (m *MemPool[VotePower]) storeTransaction(transaction mempool.Transaction, blockID iotago.BlockID) (storedTransaction *TransactionWithMetadata, isNew bool, err error) {
	m.evictionMutex.RLock()
	defer m.evictionMutex.RUnlock()

	if m.lastEvictedSlot >= blockID.Index() {
		return nil, false, xerrors.Errorf("blockID %d is older than last evicted slot %d", blockID.Index(), m.lastEvictedSlot)
	}

	newTransaction, err := NewTransactionWithMetadata(transaction)
	if err != nil {
		return nil, false, xerrors.Errorf("failed to create transaction metadata: %w", err)
	}

	storedTransaction, isNew = m.cachedTransactions.GetOrCreate(newTransaction.ID(), func() *TransactionWithMetadata {
		newTransaction.lifecycle.setStored()

		return newTransaction
	})

	storedTransaction.attachments.Add(blockID)
	m.attachments.Get(blockID.Index(), true).Set(blockID, storedTransaction)

	return storedTransaction, isNew, nil
}

func (m *MemPool[VotePower]) solidifyInputs(transaction *TransactionWithMetadata) {
	for i, inputReference := range transaction.inputReferences {
		stateReference, index := inputReference, i

		request, created := m.cachedStateRequests.GetOrCreate(stateReference.StateID(), func() *promise.Promise[*StateWithMetadata] {
			return m.requestStateWithMetadata(stateReference, true)
		})

		request.OnSuccess(func(input *StateWithMetadata) {
			transaction.publishInput(index, input)

			if created {
				m.setupStateLifecycle(input)
			}
		})

		request.OnError(transaction.lifecycle.setInvalid)
	}
}

func (m *MemPool[VotePower]) executeTransaction(transaction *TransactionWithMetadata) {
	m.executionWorkers.Submit(func() {
		if outputStates, err := m.executeStateTransition(transaction.Transaction(), lo.Map(transaction.inputs, (*StateWithMetadata).State), context.Background()); err != nil {
			transaction.lifecycle.setInvalid(err)
		} else {
			transaction.setExecuted(outputStates)
		}
	})
}

func (m *MemPool[VotePower]) bookTransaction(transaction *TransactionWithMetadata) {
	lo.ForEach(transaction.inputs, func(input *StateWithMetadata) {
		input.spentState.OnDoubleSpent(func() {
			m.forkTransaction(transaction, input)
		})
	})

	transaction.lifecycle.setBooked()
}

func (m *MemPool[VotePower]) publishOutputs(transaction *TransactionWithMetadata) {
	for _, output := range transaction.outputs {
		outputRequest, isNew := m.cachedStateRequests.GetOrCreate(output.id, lo.NoVariadic(promise.New[*StateWithMetadata]))
		outputRequest.Resolve(output)

		if isNew {
			m.setupStateLifecycle(output)
		}
	}
}

func (m *MemPool[VotePower]) requestStateWithMetadata(stateReference ledger.StateReference, waitIfMissing ...bool) *promise.Promise[*StateWithMetadata] {
	return promise.New[*StateWithMetadata](func(p *promise.Promise[*StateWithMetadata]) {
		request := m.requestInput(stateReference)

		request.OnSuccess(func(state ledger.State) {
			stateWithMetadata := NewStateWithMetadata(state)
			stateWithMetadata.inclusionState.setAccepted()
			stateWithMetadata.inclusionState.setCommitted()

			p.Resolve(stateWithMetadata)
		})

		request.OnError(func(err error) {
			if !lo.First(waitIfMissing) || !errors.Is(err, ledger.ErrStateNotFound) {
				p.Reject(err)
			}
		})
	})
}

func (m *MemPool[VotePower]) forkTransaction(transaction *TransactionWithMetadata, input *StateWithMetadata) {
	switch err := m.conflictDAG.CreateConflict(transaction.ID(), transaction.conflictIDs, advancedset.New(input.ID()), acceptance.Pending); {
	case errors.Is(err, conflictdag.ErrConflictExists):
		m.conflictDAG.JoinConflictSets(transaction.ID(), advancedset.New(input.ID()))
	case err != nil:
		panic(err)
	default:
		//transaction.setConflicting()
	}
}

func (m *MemPool[VotePower]) updateAttachment(blockID iotago.BlockID, updateFunc func(transaction *TransactionWithMetadata, blockID iotago.BlockID) bool) bool {
	m.evictionMutex.RLock()
	defer m.evictionMutex.RUnlock()

	if m.lastEvictedSlot < blockID.Index() {
		if transaction, exists := m.transactionByAttachment(blockID); exists {
			return updateFunc(transaction, blockID)
		}
	}

	return false
}

func (m *MemPool[VotePower]) transactionByAttachment(blockID iotago.BlockID) (*TransactionWithMetadata, bool) {
	if attachmentsInSlot := m.attachments.Get(blockID.Index()); attachmentsInSlot != nil {
		return attachmentsInSlot.Get(blockID)
	}

	return nil, false
}

func (m *MemPool[VotePower]) updateAcceptance(transaction *TransactionWithMetadata) {
	if transaction.AllInputsAccepted() && transaction.Attachments().WasIncluded() && m.conflictDAG.AcceptanceState(transaction.conflictIDs).IsAccepted() {
		transaction.inclusion.setAccepted()
	}
}

func (m *MemPool[VotePower]) updateStateDiffs(transaction *TransactionWithMetadata, prevIndex iotago.SlotIndex, newIndex iotago.SlotIndex) {
	if prevIndex == newIndex {
		return
	}

	if prevIndex != 0 {
		if prevSlot, exists := m.stateDiffs.Get(prevIndex); exists {
			prevSlot.RollbackTransaction(transaction)
		}
	}

	if newIndex != 0 {
		lo.Return1(m.stateDiffs.GetOrCreate(newIndex, func() *StateDiff { return NewStateDiff(newIndex) })).AddTransaction(transaction)
	}
}

var _ mempool.MemPool[vote.MockedPower] = new(MemPool[vote.MockedPower])
