package mempoolv1

import (
	"context"

	"github.com/iotaledger/hive.go/core/memstorage"
	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/hive.go/runtime/workerpool"
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

	// resolveState is the function that is used to request state from the ledger.
	resolveState mempool.StateReferenceResolver

	// attachments is the storage that is used to keep track of the attachments of transactions.
	attachments *memstorage.IndexedStorage[iotago.SlotIndex, iotago.BlockID, *TransactionMetadata]

	// cachedTransactions holds the transactions that are currently in the MemPool.
	cachedTransactions *shrinkingmap.ShrinkingMap[iotago.TransactionID, *TransactionMetadata]

	// cachedStateRequests holds the requests for states that are required to execute transactions.
	cachedStateRequests *shrinkingmap.ShrinkingMap[iotago.Identifier, *promise.Promise[mempool.State]]

	// stateDiffs holds aggregated state mutations for each slot index.
	stateDiffs *shrinkingmap.ShrinkingMap[iotago.SlotIndex, *StateDiff]

	// conflictDAG is the DAG that is used to keep track of the conflicts between transactions.
	conflictDAG conflictdag.ConflictDAG[iotago.TransactionID, iotago.OutputID, VoteRank]

	errorHandler func(error)

	// executionWorkers is the worker pool that is used to execute the state transitions of transactions.
	executionWorkers *workerpool.WorkerPool

	// lastEvictedSlot is the last slot index that was evicted from the MemPool.
	lastEvictedSlot iotago.SlotIndex

	// evictionMutex is used to synchronize the eviction of slots.
	evictionMutex syncutils.RWMutex

	optForkAllTransactions bool

	apiProvider iotago.APIProvider
}

// New is the constructor of the MemPool.
func New[VoteRank conflictdag.VoteRankType[VoteRank]](vm mempool.VM, inputResolver mempool.StateReferenceResolver, workers *workerpool.Group, conflictDAG conflictdag.ConflictDAG[iotago.TransactionID, iotago.OutputID, VoteRank], apiProvider iotago.APIProvider, errorHandler func(error), opts ...options.Option[MemPool[VoteRank]]) *MemPool[VoteRank] {
	return options.Apply(&MemPool[VoteRank]{
		transactionAttached:    event.New1[mempool.TransactionMetadata](),
		executeStateTransition: vm,
		resolveState:           inputResolver,
		attachments:            memstorage.NewIndexedStorage[iotago.SlotIndex, iotago.BlockID, *TransactionMetadata](),
		cachedTransactions:     shrinkingmap.New[iotago.TransactionID, *TransactionMetadata](),
		cachedStateRequests:    shrinkingmap.New[iotago.Identifier, *promise.Promise[mempool.State]](),
		stateDiffs:             shrinkingmap.New[iotago.SlotIndex, *StateDiff](),
		executionWorkers:       workers.CreatePool("executionWorkers", 1),
		conflictDAG:            conflictDAG,
		apiProvider:            apiProvider,
		errorHandler:           errorHandler,
	}, opts, (*MemPool[VoteRank]).setup)
}

// AttachTransaction adds a transaction to the MemPool that was attached by the given block.
func (m *MemPool[VoteRank]) AttachTransaction(transaction mempool.Transaction, blockID iotago.BlockID) (metadata mempool.TransactionMetadata, err error) {
	storedTransaction, isNew, err := m.storeTransaction(transaction, blockID)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to store transaction")
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

// OutputStateMetadata returns the metadata of the output state with the given ID.
func (m *MemPool[VoteRank]) OutputStateMetadata(stateReference *iotago.UTXOInput) (state mempool.OutputStateMetadata, err error) {
	stateRequest, exists := m.cachedStateRequests.Get(stateReference.StateID())

	if !exists || !stateRequest.WasCompleted() {
		stateRequest = m.requestState(stateReference)
	}

	stateRequest.OnSuccess(func(loadedState mempool.State) {
		switch loadedState.Type() {
		case iotago.InputUTXO:
			//nolint:forcetypeassert // we can safely assume that this is an OutputStateMetadata
			state = loadedState.(mempool.OutputStateMetadata)
		default:
			// as we are waiting for this Promise completion, we can assign the outer err
			err = ierrors.Errorf("invalid state type, only UTXO states can return metadata")
		}
	})
	stateRequest.OnError(func(requestErr error) { err = ierrors.Errorf("failed to request state: %w", requestErr) })
	stateRequest.WaitComplete()

	return state, err
}

// PublishCommitmentState publishes the given commitment state to the MemPool.
func (m *MemPool[VoteRank]) PublishCommitmentState(commitment *iotago.Commitment) {
	if stateRequest, exists := m.cachedStateRequests.Get(commitment.StateID()); exists {
		stateRequest.Resolve(commitment)
	}
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
		// block will be retained as invalid, we do not store tx failure as it was block's fault
		return nil, false, ierrors.Errorf("blockID %d is older than last evicted slot %d", blockID.Index(), m.lastEvictedSlot)
	}

	newTransaction, err := NewTransactionWithMetadata(m.apiProvider.APIForSlot(blockID.Index()), transaction)
	if err != nil {
		return nil, false, ierrors.Errorf("failed to create transaction metadata: %w", err)
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

		request, created := m.cachedStateRequests.GetOrCreate(stateReference.StateID(), func() *promise.Promise[mempool.State] {
			return m.requestState(stateReference, true)
		})

		request.OnSuccess(func(state mempool.State) {
			switch state.Type() {
			case iotago.InputUTXO:
				//nolint:forcetypeassert // we can safely assume that this is an OutputStateMetadata
				outputStateMetadata := state.(*OutputStateMetadata)

				transaction.publishInput(index, outputStateMetadata)

				if created {
					m.setupOutputState(outputStateMetadata)
				}
			case iotago.InputCommitment:
				//nolint:forcetypeassert // we can safely assume that this is an ContextStateMetadata
				contextStateMetadata := state.(*ContextStateMetadata)

				transaction.publishCommitmentInput(contextStateMetadata)

				if created {
					contextStateMetadata.onAllSpendersRemoved(func() { m.cachedStateRequests.Delete(contextStateMetadata.StateID()) })
				}
			case iotago.InputBlockIssuanceCredit, iotago.InputReward:
			default:
				panic(ierrors.New("invalid state type resolved"))
			}

			// an input has been successfully resolved, decrease the unsolid input counter and check solidity.
			if transaction.markInputSolid() {
				m.executeTransaction(transaction)
			}
		})

		request.OnError(transaction.setInvalid)
	}
}

func (m *MemPool[VoteRank]) executeTransaction(transaction *TransactionMetadata) {
	m.executionWorkers.Submit(func() {
		var timeReference mempool.ContextState
		if commitmentStateMetadata, ok := transaction.CommitmentInput().(*ContextStateMetadata); ok && commitmentStateMetadata != nil {
			timeReference = commitmentStateMetadata.State()
		}

		if outputStates, err := m.executeStateTransition(context.Background(), transaction.Transaction(), lo.Map(transaction.utxoInputs, (*OutputStateMetadata).State), timeReference); err != nil {
			transaction.setInvalid(err)
		} else {
			transaction.setExecuted(outputStates)

			m.bookTransaction(transaction)
		}
	})
}

func (m *MemPool[VoteRank]) bookTransaction(transaction *TransactionMetadata) {
	if m.optForkAllTransactions {
		m.forkTransaction(transaction, ds.NewSet(lo.Map(transaction.utxoInputs, (*OutputStateMetadata).OutputID)...))
	} else {
		lo.ForEach(transaction.utxoInputs, func(input *OutputStateMetadata) {
			input.OnDoubleSpent(func() {
				m.forkTransaction(transaction, ds.NewSet(input.OutputID()))
			})
		})
	}

	if !transaction.IsOrphaned() && transaction.setBooked() {
		m.publishOutputStates(transaction)
	}
}

func (m *MemPool[VoteRank]) forkTransaction(transaction *TransactionMetadata, resourceIDs ds.Set[iotago.OutputID]) {
	transaction.setConflicting()

	if err := m.conflictDAG.UpdateConflictingResources(transaction.ID(), resourceIDs); err != nil {
		transaction.setOrphaned()

		m.errorHandler(err)
	}
}

func (m *MemPool[VoteRank]) publishOutputStates(transaction *TransactionMetadata) {
	for _, output := range transaction.outputs {
		stateRequest, isNew := m.cachedStateRequests.GetOrCreate(output.StateID(), lo.NoVariadic(promise.New[mempool.State]))
		stateRequest.Resolve(output)

		if isNew {
			m.setupOutputState(output)
		}
	}
}

func (m *MemPool[VoteRank]) requestState(stateRef iotago.Input, waitIfMissing ...bool) *promise.Promise[mempool.State] {
	return promise.New(func(p *promise.Promise[mempool.State]) {
		request := m.resolveState(stateRef)

		request.OnSuccess(func(state mempool.State) {
			switch state.Type() {
			case iotago.InputUTXO:
				// The output was resolved from the ledger, meaning it was actually persisted as it was accepted and
				// committed: otherwise we would have found it in cache or the request would have never resolved.
				//nolint:forcetypeassert // we can safely assume that this is an OutputState
				outputStateMetadata := NewOutputStateMetadata(state.(mempool.OutputState))
				outputStateMetadata.setAccepted()
				outputStateMetadata.setCommitted()

				p.Resolve(outputStateMetadata)
			case iotago.InputCommitment:
				//nolint:forcetypeassert // we can safely assume that this is an ContextState
				commitmentStateMetadata := NewContextStateMetadata(state.(mempool.ContextState))

				p.Resolve(commitmentStateMetadata)
			case iotago.InputBlockIssuanceCredit, iotago.InputReward:
				p.Resolve(state)
			default:
				p.Reject(ierrors.Errorf("unsupported input type %s", stateRef.Type()))
			}
		})

		request.OnError(func(err error) {
			// do not reject the outer promise if the state was not found and the caller wants to wait for it
			if !lo.First(waitIfMissing) || !ierrors.Is(err, mempool.ErrStateNotFound) {
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

func (m *MemPool[VoteRank]) updateStateDiffs(transaction *TransactionMetadata, prevIndex iotago.SlotIndex, newIndex iotago.SlotIndex) error {
	if prevIndex == newIndex {
		return nil
	}

	if prevIndex != 0 {
		if prevSlot, exists := m.stateDiffs.Get(prevIndex); exists {
			if err := prevSlot.RollbackTransaction(transaction); err != nil {
				return ierrors.Wrapf(err, "failed to rollback transaction, txID: %s", transaction.ID())
			}
		}
	}

	if transaction.IsAccepted() && newIndex != 0 {
		if stateDiff, evicted := m.stateDiff(newIndex); !evicted {
			if err := stateDiff.AddTransaction(transaction, m.errorHandler); err != nil {
				return ierrors.Wrapf(err, "failed to add transaction to state diff, txID: %s", transaction.ID())
			}
		}
	}

	return nil
}

func (m *MemPool[VoteRank]) setup() {
	m.conflictDAG.Events().ConflictAccepted.Hook(func(id iotago.TransactionID) {
		if transaction, exists := m.cachedTransactions.Get(id); exists {
			transaction.setConflictAccepted()
		}
	})
}

func (m *MemPool[VoteRank]) stateDiff(slotIndex iotago.SlotIndex) (stateDiff *StateDiff, evicted bool) {
	if m.lastEvictedSlot >= slotIndex {
		return nil, true
	}

	return lo.Return1(m.stateDiffs.GetOrCreate(slotIndex, func() *StateDiff { return NewStateDiff(slotIndex) })), false
}

func (m *MemPool[VoteRank]) setupTransaction(transaction *TransactionMetadata) {
	transaction.OnAccepted(func() {
		if slotIndex := transaction.EarliestIncludedAttachment().Index(); slotIndex > 0 {
			if stateDiff, evicted := m.stateDiff(slotIndex); !evicted {
				if err := stateDiff.AddTransaction(transaction, m.errorHandler); err != nil {
					m.errorHandler(ierrors.Wrapf(err, "failed to add transaction to state diff, txID: %s", transaction.ID()))
				}
			}
		}
	})

	transaction.OnConflicting(func() {
		m.conflictDAG.CreateConflict(transaction.ID())

		unsubscribe := transaction.parentConflictIDs.OnUpdate(func(appliedMutations ds.SetMutations[iotago.TransactionID]) {
			if err := m.conflictDAG.UpdateConflictParents(transaction.ID(), appliedMutations.AddedElements(), appliedMutations.DeletedElements()); err != nil {
				panic(err)
			}
		})

		transaction.OnEvicted(func() {
			unsubscribe()

			m.conflictDAG.EvictConflict(transaction.ID())
		})
	})

	transaction.OnEarliestIncludedAttachmentUpdated(func(prevBlock, newBlock iotago.BlockID) {
		if err := m.updateStateDiffs(transaction, prevBlock.Index(), newBlock.Index()); err != nil {
			m.errorHandler(ierrors.Wrap(err, "failed to update state diffs"))
		}
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

func (m *MemPool[VoteRank]) setupOutputState(state *OutputStateMetadata) {
	state.OnCommitted(func() {
		if !m.cachedStateRequests.Delete(state.StateID(), state.HasNoSpenders) && m.cachedStateRequests.Has(state.StateID()) {
			state.onAllSpendersRemoved(func() { m.cachedStateRequests.Delete(state.StateID(), state.HasNoSpenders) })
		}
	})

	state.OnOrphaned(func() {
		m.cachedStateRequests.Delete(state.StateID())
	})
}

func WithForkAllTransactions[VoteRank conflictdag.VoteRankType[VoteRank]](forkAllTransactions bool) options.Option[MemPool[VoteRank]] {
	return func(m *MemPool[VoteRank]) {
		m.optForkAllTransactions = forkAllTransactions
	}
}

var _ mempool.MemPool[vote.MockedRank] = new(MemPool[vote.MockedRank])
