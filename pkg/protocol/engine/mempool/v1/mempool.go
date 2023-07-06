package mempoolv1

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"golang.org/x/xerrors"

	"github.com/iotaledger/hive.go/core/memstorage"
	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/core/agential"
	"github.com/iotaledger/iota-core/pkg/core/api"
	"github.com/iotaledger/iota-core/pkg/core/promise"
	"github.com/iotaledger/iota-core/pkg/core/vote"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/conflictdag"
	iotago "github.com/iotaledger/iota.go/v4"
)

// MemPool is a component that manages the state of transactions that are not yet included in the ledger state.
type MemPool[VoteRank conflictdag.VoteRankType[VoteRank]] struct {
	transactionAttached *event.Event1[mempool.TransactionMetadata]

	// executeStateTransition is the VM that is used to execute the state transition of transactions.
	executeStateTransition mempool.VM

	// requestInput is the function that is used to request state from the ledger.
	requestInput mempool.StateReferenceResolver

	// attachments is the storage that is used to keep track of the attachments of transactions.
	attachments *memstorage.IndexedStorage[iotago.SlotIndex, iotago.BlockID, *TransactionMetadata]

	// cachedTransactions holds the transactions that are currently in the MemPool.
	cachedTransactions *shrinkingmap.ShrinkingMap[iotago.TransactionID, *TransactionMetadata]

	// cachedStateRequests holds the requests for states that are required to execute transactions.
	cachedStateRequests *shrinkingmap.ShrinkingMap[iotago.OutputID, *promise.Promise[*StateMetadata]]

	// stateDiffs holds aggregated state mutations for each slot index.
	stateDiffs *shrinkingmap.ShrinkingMap[iotago.SlotIndex, *StateDiff]

	// conflictDAG is the DAG that is used to keep track of the conflicts between transactions.
	conflictDAG conflictdag.ConflictDAG[iotago.TransactionID, iotago.OutputID, VoteRank]

	// executionWorkers is the worker pool that is used to execute the state transitions of transactions.
	executionWorkers *workerpool.WorkerPool

	// lastEvictedSlot is the last slot index that was evicted from the MemPool.
	lastEvictedSlot iotago.SlotIndex

	// evictionMutex is used to synchronize the eviction of slots.
	evictionMutex sync.RWMutex

	optForkAllTransactions bool

	apiProvider api.Provider
}

// New is the constructor of the MemPool.
func New[VoteRank conflictdag.VoteRankType[VoteRank]](vm mempool.VM, inputResolver mempool.StateReferenceResolver, workers *workerpool.Group, conflictDAG conflictdag.ConflictDAG[iotago.TransactionID, iotago.OutputID, VoteRank], apiProvider api.Provider, opts ...options.Option[MemPool[VoteRank]]) *MemPool[VoteRank] {
	return options.Apply(&MemPool[VoteRank]{
		transactionAttached:    event.New1[mempool.TransactionMetadata](),
		executeStateTransition: vm,
		requestInput:           inputResolver,
		attachments:            memstorage.NewIndexedStorage[iotago.SlotIndex, iotago.BlockID, *TransactionMetadata](),
		cachedTransactions:     shrinkingmap.New[iotago.TransactionID, *TransactionMetadata](),
		cachedStateRequests:    shrinkingmap.New[iotago.OutputID, *promise.Promise[*StateMetadata]](),
		stateDiffs:             shrinkingmap.New[iotago.SlotIndex, *StateDiff](),
		executionWorkers:       workers.CreatePool("executionWorkers", 1),
		conflictDAG:            conflictDAG,
		apiProvider:            apiProvider,
	}, opts, (*MemPool[VoteRank]).setup)
}

// AttachTransaction adds a transaction to the MemPool that was attached by the given block.
func (m *MemPool[VoteRank]) AttachTransaction(transaction mempool.Transaction, blockID iotago.BlockID) (metadata mempool.TransactionMetadata, err error) {
	storedTransaction, isNew, err := m.storeTransaction(transaction, blockID)
	if err != nil {
		return nil, xerrors.Errorf("failed to store transaction: %w", err)
	}

	if isNew {
		m.transactionAttached.Trigger(storedTransaction)

		m.solidifyInputs(storedTransaction)
	}

	return storedTransaction, nil
}

func (m *MemPool[VoteRank]) OnTransactionAttached(handler func(transaction mempool.TransactionMetadata), opts ...event.Option) {
	m.transactionAttached.Hook(handler, opts...)
}

// MarkAttachmentOrphaned marks the attachment of the given block as orphaned.
func (m *MemPool[VoteRank]) MarkAttachmentOrphaned(blockID iotago.BlockID) bool {
	if attachmentSlot := m.attachments.Get(blockID.Index(), false); attachmentSlot != nil {
		defer attachmentSlot.Delete(blockID)
	}

	return m.updateAttachment(blockID, (*TransactionMetadata).markAttachmentOrphaned)
}

// MarkAttachmentIncluded marks the attachment of the given block as included.
func (m *MemPool[VoteRank]) MarkAttachmentIncluded(blockID iotago.BlockID) bool {
	return m.updateAttachment(blockID, (*TransactionMetadata).markAttachmentIncluded)
}

// TransactionMetadata returns the metadata of the transaction with the given ID.
func (m *MemPool[VoteRank]) TransactionMetadata(id iotago.TransactionID) (transaction mempool.TransactionMetadata, exists bool) {
	return m.cachedTransactions.Get(id)
}

// StateMetadata returns the metadata of the state with the given ID.
func (m *MemPool[VoteRank]) StateMetadata(stateReference iotago.IndexedUTXOReferencer) (state mempool.StateMetadata, err error) {
	stateRequest, exists := m.cachedStateRequests.Get(stateReference.Ref())
	if !exists || !stateRequest.WasCompleted() {
		stateRequest = m.requestStateWithMetadata(stateReference)
	}

	stateRequest.OnSuccess(func(loadedState *StateMetadata) { state = loadedState })
	stateRequest.OnError(func(requestErr error) { err = xerrors.Errorf("failed to request state: %w", requestErr) })
	stateRequest.WaitComplete()

	return state, err
}

// TransactionMetadataByAttachment returns the metadata of the transaction that was attached by the given block.
func (m *MemPool[VoteRank]) TransactionMetadataByAttachment(blockID iotago.BlockID) (mempool.TransactionMetadata, bool) {
	return m.transactionByAttachment(blockID)
}

// StateDiff returns the state diff for the given slot index.
func (m *MemPool[VoteRank]) StateDiff(index iotago.SlotIndex) mempool.StateDiff {
	if stateDiff, exists := m.stateDiffs.Get(index); exists {
		return stateDiff
	}

	return NewStateDiff(index)
}

// Evict evicts the slot with the given index from the MemPool.
func (m *MemPool[VoteRank]) Evict(slotIndex iotago.SlotIndex) {
	if evictedAttachments := func() *shrinkingmap.ShrinkingMap[iotago.BlockID, *TransactionMetadata] {
		m.evictionMutex.Lock()
		defer m.evictionMutex.Unlock()

		m.lastEvictedSlot = slotIndex

		m.stateDiffs.Delete(slotIndex)

		return m.attachments.Evict(slotIndex)
	}(); evictedAttachments != nil {
		evictedAttachments.ForEach(func(blockID iotago.BlockID, transaction *TransactionMetadata) bool {
			transaction.evictAttachment(blockID)

			return true
		})
	}
}

func (m *MemPool[VoteRank]) storeTransaction(transaction mempool.Transaction, blockID iotago.BlockID) (storedTransaction *TransactionMetadata, isNew bool, err error) {
	m.evictionMutex.RLock()
	defer m.evictionMutex.RUnlock()

	if m.lastEvictedSlot >= blockID.Index() {
		return nil, false, xerrors.Errorf("blockID %d is older than last evicted slot %d", blockID.Index(), m.lastEvictedSlot)
	}

	newTransaction, err := NewTransactionWithMetadata(m.apiProvider.APIForSlot(blockID.Index()), transaction)
	if err != nil {
		return nil, false, xerrors.Errorf("failed to create transaction metadata: %w", err)
	}

	storedTransaction, isNew = m.cachedTransactions.GetOrCreate(newTransaction.ID(), func() *TransactionMetadata { return newTransaction })
	if isNew {
		m.setupTransaction(storedTransaction)
	}

	storedTransaction.addAttachment(blockID)
	m.attachments.Get(blockID.Index(), true).Set(blockID, storedTransaction)

	return storedTransaction, isNew, nil
}

func (m *MemPool[VoteRank]) solidifyInputs(transaction *TransactionMetadata) {
	for i, inputReference := range transaction.inputReferences {
		stateReference, index := inputReference, i

		request, created := m.cachedStateRequests.GetOrCreate(stateReference.Ref(), func() *promise.Promise[*StateMetadata] {
			return m.requestStateWithMetadata(stateReference, true)
		})

		request.OnSuccess(func(input *StateMetadata) {
			if transaction.publishInputAndCheckSolidity(index, input) {
				m.executeTransaction(transaction)
			}

			if created {
				m.setupState(input)
			}
		})

		request.OnError(transaction.setInvalid)
	}
}

func (m *MemPool[VoteRank]) executeTransaction(transaction *TransactionMetadata) {
	m.executionWorkers.Submit(func() {
		if outputStates, err := m.executeStateTransition(context.Background(), transaction.Transaction(), lo.Map(transaction.inputs, (*StateMetadata).State)); err != nil {
			transaction.setInvalid(err)
		} else {
			transaction.setExecuted(outputStates)

			m.bookTransaction(transaction)
		}
	})
}

func (m *MemPool[VoteRank]) bookTransaction(transaction *TransactionMetadata) {
	if m.optForkAllTransactions {
		m.forkTransaction(transaction, advancedset.New(lo.Map(transaction.inputs, (*StateMetadata).ID)...))
	} else {
		lo.ForEach(transaction.inputs, func(input *StateMetadata) {
			input.OnDoubleSpent(func() {
				m.forkTransaction(transaction, advancedset.New(input.ID()))
			})
		})
	}

	if !transaction.IsOrphaned() && transaction.setBooked() {
		m.publishOutputs(transaction)
	}
}

func (m *MemPool[VoteRank]) forkTransaction(transaction *TransactionMetadata, resourceIDs *advancedset.AdvancedSet[iotago.OutputID]) {
	transaction.setConflicting()

	if err := m.conflictDAG.UpdateConflictingResources(transaction.ID(), resourceIDs); err != nil {
		transaction.setOrphaned()

		// TODO: use errorHandler mechanism instead.
		fmt.Println(err)
	}
}

func (m *MemPool[VoteRank]) publishOutputs(transaction *TransactionMetadata) {
	for _, output := range transaction.outputs {
		outputRequest, isNew := m.cachedStateRequests.GetOrCreate(output.id, lo.NoVariadic(promise.New[*StateMetadata]))
		outputRequest.Resolve(output)

		if isNew {
			m.setupState(output)
		}
	}
}

func (m *MemPool[VoteRank]) requestStateWithMetadata(stateRef iotago.IndexedUTXOReferencer, waitIfMissing ...bool) *promise.Promise[*StateMetadata] {
	return promise.New(func(p *promise.Promise[*StateMetadata]) {
		request := m.requestInput(stateRef)

		request.OnSuccess(func(state mempool.State) {
			stateWithMetadata := NewStateMetadata(state)
			stateWithMetadata.setAccepted()
			stateWithMetadata.setCommitted()

			p.Resolve(stateWithMetadata)
		})

		request.OnError(func(err error) {
			if !lo.First(waitIfMissing) || !errors.Is(err, mempool.ErrStateNotFound) {
				p.Reject(err)
			}
		})
	})
}

func (m *MemPool[VoteRank]) updateAttachment(blockID iotago.BlockID, updateFunc func(transaction *TransactionMetadata, blockID iotago.BlockID) bool) bool {
	m.evictionMutex.RLock()
	defer m.evictionMutex.RUnlock()

	if m.lastEvictedSlot < blockID.Index() {
		if transaction, exists := m.transactionByAttachment(blockID); exists {
			return updateFunc(transaction, blockID)
		}
	}

	return false
}

func (m *MemPool[VoteRank]) transactionByAttachment(blockID iotago.BlockID) (*TransactionMetadata, bool) {
	if attachmentsInSlot := m.attachments.Get(blockID.Index()); attachmentsInSlot != nil {
		return attachmentsInSlot.Get(blockID)
	}

	return nil, false
}

func (m *MemPool[VoteRank]) updateStateDiffs(transaction *TransactionMetadata, prevIndex iotago.SlotIndex, newIndex iotago.SlotIndex) {
	if prevIndex == newIndex {
		return
	}

	if prevIndex != 0 {
		if prevSlot, exists := m.stateDiffs.Get(prevIndex); exists {
			prevSlot.RollbackTransaction(transaction)
		}
	}

	if transaction.IsAccepted() && newIndex != 0 {
		if stateDiff, evicted := m.stateDiff(newIndex); !evicted {
			stateDiff.AddTransaction(transaction)
		}
	}
}

func (m *MemPool[VoteRank]) setup() {
	m.conflictDAG.Events().ConflictAccepted.Hook(func(id iotago.TransactionID) {
		if transaction, exists := m.cachedTransactions.Get(id); exists {
			transaction.setConflictAccepted()
		}
	})
}

func (m *MemPool[VoteRank]) stateDiff(slotIndex iotago.SlotIndex) (stateDiff *StateDiff, evicted bool) {
	m.evictionMutex.RLock()
	defer m.evictionMutex.RUnlock()

	if m.lastEvictedSlot >= slotIndex {
		return nil, true
	}

	return lo.Return1(m.stateDiffs.GetOrCreate(slotIndex, func() *StateDiff { return NewStateDiff(slotIndex) })), false
}

func (m *MemPool[VoteRank]) setupTransaction(transaction *TransactionMetadata) {
	transaction.OnAccepted(func() {
		if slotIndex := transaction.EarliestIncludedAttachment().Index(); slotIndex > 0 {
			if stateDiff, evicted := m.stateDiff(slotIndex); !evicted {
				stateDiff.AddTransaction(transaction)
			}
		}
	})

	transaction.OnConflicting(func() {
		m.conflictDAG.CreateConflict(transaction.ID())

		unsubscribe := transaction.parentConflictIDs.OnUpdate(func(_ *advancedset.AdvancedSet[iotago.TransactionID], appliedMutations *agential.SetReceptorMutations[iotago.TransactionID]) {
			if err := m.conflictDAG.UpdateConflictParents(transaction.ID(), appliedMutations.AddedElements, appliedMutations.RemovedElements); err != nil {
				panic(err)
			}
		})

		transaction.OnEvicted(func() {
			unsubscribe()

			m.conflictDAG.EvictConflict(transaction.ID())
		})

	})

	transaction.OnEarliestIncludedAttachmentUpdated(func(prevBlock, newBlock iotago.BlockID) {
		m.updateStateDiffs(transaction, prevBlock.Index(), newBlock.Index())
	})

	transaction.OnEvicted(func() {
		if m.cachedTransactions.Delete(transaction.ID()) {
			transaction.attachments.ForEach(func(blockID iotago.BlockID, _ bool) bool {
				if slotAttachments := m.attachments.Get(blockID.Index(), false); slotAttachments != nil {
					slotAttachments.Delete(blockID)
				}

				return true
			})
		}
	})
}

func (m *MemPool[VoteRank]) setupState(state *StateMetadata) {
	state.OnCommitted(func() {
		if !m.cachedStateRequests.Delete(state.ID(), state.HasNoSpenders) && m.cachedStateRequests.Has(state.ID()) {
			state.onAllSpendersRemoved(func() { m.cachedStateRequests.Delete(state.ID(), state.HasNoSpenders) })
		}
	})

	state.OnOrphaned(func() {
		m.cachedStateRequests.Delete(state.ID())
	})
}

func WithForkAllTransactions[VoteRank conflictdag.VoteRankType[VoteRank]](forkAllTransactions bool) options.Option[MemPool[VoteRank]] {
	return func(m *MemPool[VoteRank]) {
		m.optForkAllTransactions = forkAllTransactions
	}
}

var _ mempool.MemPool[vote.MockedRank] = new(MemPool[vote.MockedRank])
