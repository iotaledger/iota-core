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
	// vm is the virtual machine that is used to validate and execute transactions.
	vm mempool.VM

	// resolveState is the function that is used to request state from the ledger.
	resolveState mempool.StateResolver

	// attachments is the storage that is used to keep track of the attachments of transactions.
	attachments *memstorage.IndexedStorage[iotago.SlotIndex, iotago.BlockID, *SignedTransactionMetadata]

	// cachedTransactions holds the transactions that are currently in the MemPool.
	cachedTransactions *shrinkingmap.ShrinkingMap[iotago.TransactionID, *TransactionMetadata]

	// cachedSignedTransactions holds the signed transactions that are currently in the MemPool.
	cachedSignedTransactions *shrinkingmap.ShrinkingMap[iotago.SignedTransactionID, *SignedTransactionMetadata]

	// cachedStateRequests holds the requests for states that are required to execute transactions.
	cachedStateRequests *shrinkingmap.ShrinkingMap[mempool.StateID, *promise.Promise[*StateMetadata]]

	// stateDiffs holds aggregated state mutations for each slot.
	stateDiffs *shrinkingmap.ShrinkingMap[iotago.SlotIndex, *StateDiff]

	// delayedTransactionEviction holds the transactions that can only be evicted after MaxCommittableAge to objectively
	// invalidate blocks that try to spend from them.
	delayedTransactionEviction *shrinkingmap.ShrinkingMap[iotago.SlotIndex, ds.Set[iotago.TransactionID]]

	// delayedOutputStateEviction holds the outputs that can only be evicted after MaxCommittableAge to objectively
	// invalidate blocks that try to spend them.
	delayedOutputStateEviction *shrinkingmap.ShrinkingMap[iotago.SlotIndex, *shrinkingmap.ShrinkingMap[iotago.Identifier, *StateMetadata]]

	// conflictDAG is the DAG that is used to keep track of the conflicts between transactions.
	conflictDAG conflictdag.ConflictDAG[iotago.TransactionID, mempool.StateID, VoteRank]

	apiProvider iotago.APIProvider

	errorHandler func(error)

	// executionWorkers is the worker pool that is used to execute the state transitions of transactions.
	executionWorkers *workerpool.WorkerPool

	// lastEvictedSlot is the last slot that was evicted from the MemPool.
	lastEvictedSlot iotago.SlotIndex

	// evictionMutex is used to synchronize the eviction of slots.
	evictionMutex syncutils.RWMutex

	optForkAllTransactions bool

	signedTransactionAttached *event.Event1[mempool.SignedTransactionMetadata]

	transactionAttached *event.Event1[mempool.TransactionMetadata]
}

// New is the constructor of the MemPool.
func New[VoteRank conflictdag.VoteRankType[VoteRank]](
	vm mempool.VM,
	stateResolver mempool.StateResolver,
	workers *workerpool.Group,
	conflictDAG conflictdag.ConflictDAG[iotago.TransactionID, mempool.StateID, VoteRank],
	apiProvider iotago.APIProvider,
	errorHandler func(error),
	opts ...options.Option[MemPool[VoteRank]],
) *MemPool[VoteRank] {
	return options.Apply(&MemPool[VoteRank]{
		vm:                         vm,
		resolveState:               stateResolver,
		attachments:                memstorage.NewIndexedStorage[iotago.SlotIndex, iotago.BlockID, *SignedTransactionMetadata](),
		cachedTransactions:         shrinkingmap.New[iotago.TransactionID, *TransactionMetadata](),
		cachedSignedTransactions:   shrinkingmap.New[iotago.SignedTransactionID, *SignedTransactionMetadata](),
		cachedStateRequests:        shrinkingmap.New[mempool.StateID, *promise.Promise[*StateMetadata]](),
		stateDiffs:                 shrinkingmap.New[iotago.SlotIndex, *StateDiff](),
		executionWorkers:           workers.CreatePool("executionWorkers", 1),
		delayedTransactionEviction: shrinkingmap.New[iotago.SlotIndex, ds.Set[iotago.TransactionID]](),
		delayedOutputStateEviction: shrinkingmap.New[iotago.SlotIndex, *shrinkingmap.ShrinkingMap[iotago.Identifier, *StateMetadata]](),
		conflictDAG:                conflictDAG,
		apiProvider:                apiProvider,
		errorHandler:               errorHandler,
		signedTransactionAttached:  event.New1[mempool.SignedTransactionMetadata](),
		transactionAttached:        event.New1[mempool.TransactionMetadata](),
	}, opts, (*MemPool[VoteRank]).setup)
}

func (m *MemPool[VoteRank]) VM() mempool.VM {
	return m.vm
}

// AttachSignedTransaction adds a transaction to the MemPool that was attached by the given block.
func (m *MemPool[VoteRank]) AttachSignedTransaction(signedTransaction mempool.SignedTransaction, transaction mempool.Transaction, blockID iotago.BlockID) (signedTransactionMetadata mempool.SignedTransactionMetadata, err error) {
	storedSignedTransaction, isNewSignedTransaction, isNewTransaction, err := m.storeTransaction(signedTransaction, transaction, blockID)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to store signedTransaction")
	}

	if isNewSignedTransaction {
		m.signedTransactionAttached.Trigger(storedSignedTransaction)

		if isNewTransaction {
			m.transactionAttached.Trigger(storedSignedTransaction.transactionMetadata)

			m.solidifyInputs(storedSignedTransaction.transactionMetadata)
		}

	}

	return storedSignedTransaction, nil
}

func (m *MemPool[VoteRank]) OnSignedTransactionAttached(handler func(signedTransactionMetadata mempool.SignedTransactionMetadata), opts ...event.Option) {
	m.signedTransactionAttached.Hook(handler, opts...)
}

func (m *MemPool[VoteRank]) OnTransactionAttached(handler func(transaction mempool.TransactionMetadata), opts ...event.Option) {
	m.transactionAttached.Hook(handler, opts...)
}

// MarkAttachmentIncluded marks the attachment of the given block as included.
func (m *MemPool[VoteRank]) MarkAttachmentIncluded(blockID iotago.BlockID) bool {
	return m.updateAttachment(blockID, (*TransactionMetadata).markAttachmentIncluded)
}

// TransactionMetadata returns the metadata of the transaction with the given ID.
func (m *MemPool[VoteRank]) TransactionMetadata(id iotago.TransactionID) (transaction mempool.TransactionMetadata, exists bool) {
	return m.cachedTransactions.Get(id)
}

// StateMetadata returns the metadata of the output state with the given ID.
func (m *MemPool[VoteRank]) StateMetadata(stateReference mempool.StateReference) (state mempool.StateMetadata, err error) {
	stateRequest, exists := m.cachedStateRequests.Get(stateReference.ReferencedStateID())

	// create a new request that does not wait for missing states
	if !exists || !stateRequest.WasCompleted() {
		stateRequest = m.requestState(stateReference)
		stateRequest.WaitComplete()
	}

	return stateRequest.Result(), stateRequest.Err()
}

// InjectRequestedState allows to inject a requested state into the MemPool that is provided on the fly instead of being
// provided by the ledger.
func (m *MemPool[VoteRank]) InjectRequestedState(state mempool.State) {
	if stateRequest, exists := m.cachedStateRequests.Get(state.StateID()); exists {
		stateRequest.Resolve(NewStateMetadata(state))
	}
}

// TransactionMetadataByAttachment returns the metadata of the transaction that was attached by the given block.
func (m *MemPool[VoteRank]) TransactionMetadataByAttachment(blockID iotago.BlockID) (mempool.TransactionMetadata, bool) {
	return m.transactionByAttachment(blockID)
}

// StateDiff returns the state diff for the given slot.
func (m *MemPool[VoteRank]) StateDiff(slot iotago.SlotIndex) mempool.StateDiff {
	if stateDiff, exists := m.stateDiffs.Get(slot); exists {
		return stateDiff
	}

	return NewStateDiff(slot)
}

// Evict evicts the slot with the given slot from the MemPool.
func (m *MemPool[VoteRank]) Evict(slot iotago.SlotIndex) {
	if evictedAttachments := func() *shrinkingmap.ShrinkingMap[iotago.BlockID, *SignedTransactionMetadata] {
		m.evictionMutex.Lock()
		defer m.evictionMutex.Unlock()

		m.lastEvictedSlot = slot

		m.stateDiffs.Delete(slot)

		return m.attachments.Evict(slot)
	}(); evictedAttachments != nil {
		evictedAttachments.ForEach(func(blockID iotago.BlockID, signedTransactionMetadata *SignedTransactionMetadata) bool {
			signedTransactionMetadata.evictAttachment(blockID)
			return true
		})
	}

	maxCommittableAge := m.apiProvider.APIForSlot(slot).ProtocolParameters().MaxCommittableAge()
	if slot <= maxCommittableAge {
		return
	}

	delayedEvictionSlot := slot - maxCommittableAge
	if delayedTransactions, exists := m.delayedTransactionEviction.Get(delayedEvictionSlot); exists {
		delayedTransactions.Range(func(txID iotago.TransactionID) {
			if transaction, exists := m.cachedTransactions.Get(txID); exists {
				transaction.setEvicted()
			}
		})
		m.delayedTransactionEviction.Delete(delayedEvictionSlot)
	}

	if delayedOutputs, exists := m.delayedOutputStateEviction.Get(delayedEvictionSlot); exists {
		delayedOutputs.ForEach(func(stateID iotago.Identifier, state *StateMetadata) bool {
			if !m.cachedStateRequests.Delete(stateID, state.HasNoSpenders) && m.cachedStateRequests.Has(stateID) {
				state.onAllSpendersRemoved(func() { m.cachedStateRequests.Delete(stateID, state.HasNoSpenders) })
			}

			return true
		})
		m.delayedOutputStateEviction.Delete(delayedEvictionSlot)
	}
}

func (m *MemPool[VoteRank]) storeTransaction(signedTransaction mempool.SignedTransaction, transaction mempool.Transaction, blockID iotago.BlockID) (storedSignedTransaction *SignedTransactionMetadata, isNewSignedTransaction, isNewTransaction bool, err error) {
	m.evictionMutex.RLock()
	defer m.evictionMutex.RUnlock()

	if m.lastEvictedSlot >= blockID.Slot() {
		// block will be retained as invalid, we do not store tx failure as it was block's fault
		return nil, false, false, ierrors.Errorf("blockID %d is older than last evicted slot %d", blockID.Slot(), m.lastEvictedSlot)
	}

	inputReferences, err := m.vm.Inputs(transaction)
	if err != nil {
		return nil, false, false, ierrors.Wrap(err, "failed to get input references of transaction")
	}

	newTransaction, err := NewTransactionMetadata(transaction, inputReferences)
	if err != nil {
		return nil, false, false, ierrors.Errorf("failed to create transaction metadata: %w", err)
	}

	storedTransaction, isNewTransaction := m.cachedTransactions.GetOrCreate(newTransaction.ID(), func() *TransactionMetadata { return newTransaction })
	if isNewTransaction {
		m.setupTransaction(storedTransaction)
	}

	newSignedTransaction, err := NewSignedTransactionMetadata(signedTransaction, storedTransaction)
	if err != nil {
		return nil, false, false, ierrors.Errorf("failed to create signedTransaction metadata: %w", err)
	}

	storedSignedTransaction, isNewSignedTransaction = m.cachedSignedTransactions.GetOrCreate(lo.PanicOnErr(signedTransaction.ID()), func() *SignedTransactionMetadata { return newSignedTransaction })
	if isNewSignedTransaction {
		m.setupSignedTransaction(storedSignedTransaction, storedTransaction)
	}

	storedSignedTransaction.addAttachment(blockID)
	m.attachments.Get(blockID.Slot(), true).Set(blockID, storedSignedTransaction)

	return storedSignedTransaction, isNewSignedTransaction, isNewTransaction, nil
}

func (m *MemPool[VoteRank]) solidifyInputs(transaction *TransactionMetadata) {
	for i, inputReference := range transaction.inputReferences {
		stateReference, index := inputReference, i

		request, created := m.cachedStateRequests.GetOrCreate(stateReference.ReferencedStateID(), func() *promise.Promise[*StateMetadata] {
			return m.requestState(stateReference, true)
		})

		request.OnSuccess(func(inputState *StateMetadata) {
			transaction.publishInput(index, inputState)

			if created {
				m.setupOutputState(inputState)
			}

			if transaction.markInputSolid() {
				transaction.executionContext.OnUpdate(func(_, executionContext context.Context) {
					m.executeTransaction(executionContext, transaction)
				})
			}
		})

		request.OnError(transaction.setInvalid)
	}
}

func (m *MemPool[VoteRank]) executeTransaction(executionContext context.Context, transaction *TransactionMetadata) {
	m.executionWorkers.Submit(func() {
		if outputStates, err := m.vm.Execute(executionContext, transaction.Transaction()); err != nil {
			transaction.setInvalid(err)
		} else {
			transaction.setExecuted(outputStates)

			m.bookTransaction(transaction)
		}
	})
}

func (m *MemPool[VoteRank]) bookTransaction(transaction *TransactionMetadata) {
	if m.optForkAllTransactions {
		inputsToFork := lo.Filter(transaction.inputs, func(metadata *StateMetadata) bool {
			return !metadata.state.IsReadOnly()
		})

		m.forkTransaction(transaction, ds.NewSet(lo.Map(inputsToFork, func(stateMetadata *StateMetadata) mempool.StateID {
			return stateMetadata.state.StateID()
		})...))
	} else {
		lo.ForEach(transaction.inputs, func(input *StateMetadata) {
			if !input.state.IsReadOnly() {
				input.OnDoubleSpent(func() {
					m.forkTransaction(transaction, ds.NewSet(input.state.StateID()))
				})
			}
		})
	}

	// if !lo.Return2(transaction.IsOrphaned()) && transaction.setBooked() {
	if transaction.setBooked() {
		m.publishOutputStates(transaction)
	}
}

func (m *MemPool[VoteRank]) forkTransaction(transactionMetadata *TransactionMetadata, resourceIDs ds.Set[mempool.StateID]) {
	transactionMetadata.conflicting.Trigger()

	if err := m.conflictDAG.UpdateConflictingResources(transactionMetadata.ID(), resourceIDs); err != nil {
		// this is a hack, as with a reactive.Variable we cannot set it to 0 and still check if it was orphaned.
		transactionMetadata.orphaned.Set(1)

		m.errorHandler(err)
	}
}

func (m *MemPool[VoteRank]) publishOutputStates(transaction *TransactionMetadata) {
	for _, output := range transaction.outputs {
		stateRequest, isNew := m.cachedStateRequests.GetOrCreate(output.State().StateID(), lo.NoVariadic(promise.New[*StateMetadata]))
		stateRequest.Resolve(output)

		if isNew {
			m.setupOutputState(output)
		}
	}
}

func (m *MemPool[VoteRank]) requestState(stateRef mempool.StateReference, waitIfMissing ...bool) *promise.Promise[*StateMetadata] {
	return promise.New(func(p *promise.Promise[*StateMetadata]) {
		request := m.resolveState(stateRef)

		request.OnSuccess(func(state mempool.State) {
			// The output was resolved from the ledger, meaning it was actually persisted as it was accepted and
			// committed: otherwise we would have found it in cache or the request would have never resolved.
			stateMetadata := NewStateMetadata(state)
			stateMetadata.accepted.Set(true)

			p.Resolve(stateMetadata)
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

	if m.lastEvictedSlot < blockID.Slot() {
		if transaction, exists := m.transactionByAttachment(blockID); exists {
			return updateFunc(transaction, blockID)
		}
	}

	return false
}

func (m *MemPool[VoteRank]) transactionByAttachment(blockID iotago.BlockID) (*TransactionMetadata, bool) {
	if attachmentsInSlot := m.attachments.Get(blockID.Slot()); attachmentsInSlot != nil {
		if signedTransactionMetadata, exists := attachmentsInSlot.Get(blockID); exists {
			return signedTransactionMetadata.transactionMetadata, true
		}
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

func (m *MemPool[VoteRank]) stateDiff(slot iotago.SlotIndex) (stateDiff *StateDiff, evicted bool) {
	if m.lastEvictedSlot >= slot {
		return nil, true
	}

	return lo.Return1(m.stateDiffs.GetOrCreate(slot, func() *StateDiff { return NewStateDiff(slot) })), false
}

func (m *MemPool[VoteRank]) setupTransaction(transaction *TransactionMetadata) {
	transaction.OnAccepted(func() {
		if slot := transaction.EarliestIncludedAttachment().Slot(); slot != 0 {
			if stateDiff, evicted := m.stateDiff(slot); !evicted {
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
		if err := m.updateStateDiffs(transaction, prevBlock.Slot(), newBlock.Slot()); err != nil {
			m.errorHandler(ierrors.Wrap(err, "failed to update state diffs"))
		}
	})

	transaction.OnEvicted(func() {
		if m.cachedTransactions.Delete(transaction.ID()) {
			transaction.validAttachments.ForEach(func(blockID iotago.BlockID, _ bool) bool {
				if slotAttachments := m.attachments.Get(blockID.Slot(), false); slotAttachments != nil {
					slotAttachments.Delete(blockID)
				}

				return true
			})
		}

		transaction.signingTransactions.Range((*SignedTransactionMetadata).setEvicted)
	})

	transaction.OnCommitted(func(slot iotago.SlotIndex) {
		lo.Return1(m.delayedTransactionEviction.GetOrCreate(slot, func() ds.Set[iotago.TransactionID] { return ds.NewSet[iotago.TransactionID]() })).Add(transaction.ID())
	})

	transaction.OnOrphaned(func(slot iotago.SlotIndex) {
		lo.Return1(m.delayedTransactionEviction.GetOrCreate(slot, func() ds.Set[iotago.TransactionID] { return ds.NewSet[iotago.TransactionID]() })).Add(transaction.ID())
	})
}

func (m *MemPool[VoteRank]) setupOutputState(stateMetadata *StateMetadata) {
	stateMetadata.onAllSpendersRemoved(func() {
		m.cachedStateRequests.Delete(stateMetadata.state.StateID(), stateMetadata.HasNoSpenders)
	})

	stateMetadata.OnCommitted(func(slot iotago.SlotIndex) {
		lo.Return1(m.delayedOutputStateEviction.GetOrCreate(slot, func() *shrinkingmap.ShrinkingMap[iotago.Identifier, *StateMetadata] {
			return shrinkingmap.New[iotago.Identifier, *StateMetadata]()
		})).Set(stateMetadata.state.StateID(), stateMetadata)
	})

	stateMetadata.OnOrphaned(func(slot iotago.SlotIndex) {
		lo.Return1(m.delayedOutputStateEviction.GetOrCreate(slot, func() *shrinkingmap.ShrinkingMap[iotago.Identifier, *StateMetadata] {
			return shrinkingmap.New[iotago.Identifier, *StateMetadata]()
		})).Set(stateMetadata.state.StateID(), stateMetadata)
	})
}

func (m *MemPool[VoteRank]) setupSignedTransaction(signedTransactionMetadata *SignedTransactionMetadata, transaction *TransactionMetadata) {
	transaction.addSigningTransaction(signedTransactionMetadata)

	transaction.OnSolid(func() {
		executionContext, err := m.vm.ValidateSignatures(signedTransactionMetadata.SignedTransaction(), lo.Map(signedTransactionMetadata.transactionMetadata.inputs, (*StateMetadata).State))
		if err != nil {
			_ = signedTransactionMetadata.signaturesInvalid.Set(err)
			return
		}

		signedTransactionMetadata.attachments.OnUpdate(func(mutations ds.SetMutations[iotago.BlockID]) {
			mutations.AddedElements().Range(lo.Void(transaction.addValidAttachment))
			mutations.DeletedElements().Range(transaction.evictValidAttachment)
		})

		signedTransactionMetadata.signaturesValid.Trigger()

		transaction.executionContext.Set(executionContext)
	})

	signedTransactionMetadata.evicted.OnTrigger(func() {
		m.cachedSignedTransactions.Delete(signedTransactionMetadata.ID())
	})
}

func WithForkAllTransactions[VoteRank conflictdag.VoteRankType[VoteRank]](forkAllTransactions bool) options.Option[MemPool[VoteRank]] {
	return func(m *MemPool[VoteRank]) {
		m.optForkAllTransactions = forkAllTransactions
	}
}

var _ mempool.MemPool[vote.MockedRank] = new(MemPool[vote.MockedRank])
