package mempoolv1

import (
	"context"
	"fmt"

	"github.com/iotaledger/hive.go/core/memstorage"
	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/core/promise"
	"github.com/iotaledger/iota-core/pkg/core/vote"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/spenddag"
	iotago "github.com/iotaledger/iota.go/v4"
)

// MemPool is a component that manages the state of transactions that are not yet included in the ledger state.
type MemPool[VoteRank spenddag.VoteRankType[VoteRank]] struct {
	// vm is the virtual machine that is used to validate and execute transactions.
	vm mempool.VM

	// resolveState is the function that is used to request state from the ledger.
	resolveState mempool.StateResolver

	mutationsFunc func(iotago.SlotIndex) (kvstore.KVStore, error)

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

	// spendDAG is the DAG that is used to keep track of the conflicts between transactions.
	spendDAG spenddag.SpendDAG[iotago.TransactionID, mempool.StateID, VoteRank]

	apiProvider iotago.APIProvider

	errorHandler func(error)

	// lastCommittedSlot is the last slot that was committed in the MemPool.
	lastCommittedSlot iotago.SlotIndex

	// commitmentMutex is used to synchronize commitment of slots.
	commitmentMutex syncutils.RWMutex

	attachTransactionFailed *event.Event3[iotago.TransactionID, iotago.BlockID, error]

	signedTransactionAttached *event.Event1[mempool.SignedTransactionMetadata]

	transactionAttached *event.Event1[mempool.TransactionMetadata]
}

// New is the constructor of the MemPool.
func New[VoteRank spenddag.VoteRankType[VoteRank]](
	vm mempool.VM,
	stateResolver mempool.StateResolver,
	mutationsFunc func(iotago.SlotIndex) (kvstore.KVStore, error),
	spendDAG spenddag.SpendDAG[iotago.TransactionID, mempool.StateID, VoteRank],
	apiProvider iotago.APIProvider,
	errorHandler func(error),
	opts ...options.Option[MemPool[VoteRank]],
) *MemPool[VoteRank] {
	return options.Apply(&MemPool[VoteRank]{
		vm:                         vm,
		resolveState:               stateResolver,
		mutationsFunc:              mutationsFunc,
		attachments:                memstorage.NewIndexedStorage[iotago.SlotIndex, iotago.BlockID, *SignedTransactionMetadata](),
		cachedTransactions:         shrinkingmap.New[iotago.TransactionID, *TransactionMetadata](),
		cachedSignedTransactions:   shrinkingmap.New[iotago.SignedTransactionID, *SignedTransactionMetadata](),
		cachedStateRequests:        shrinkingmap.New[mempool.StateID, *promise.Promise[*StateMetadata]](),
		stateDiffs:                 shrinkingmap.New[iotago.SlotIndex, *StateDiff](),
		delayedTransactionEviction: shrinkingmap.New[iotago.SlotIndex, ds.Set[iotago.TransactionID]](),
		delayedOutputStateEviction: shrinkingmap.New[iotago.SlotIndex, *shrinkingmap.ShrinkingMap[iotago.Identifier, *StateMetadata]](),
		spendDAG:                   spendDAG,
		apiProvider:                apiProvider,
		errorHandler:               errorHandler,
		attachTransactionFailed:    event.New3[iotago.TransactionID, iotago.BlockID, error](),
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
		err = ierrors.Wrap(err, "failed to store signedTransaction")
		m.attachTransactionFailed.Trigger(transaction.MustID(), blockID, err)

		return nil, err
	}

	if isNewSignedTransaction {
		m.signedTransactionAttached.Trigger(storedSignedTransaction)

		if isNewTransaction {
			m.transactionAttached.Trigger(storedSignedTransaction.transactionMetadata)
			fmt.Println("solidify inputs of", storedSignedTransaction.transactionMetadata.ID())
			m.solidifyInputs(storedSignedTransaction.transactionMetadata)
		}
	}

	return storedSignedTransaction, nil
}

func (m *MemPool[VoteRank]) OnAttachTransactionFailed(handler func(iotago.TransactionID, iotago.BlockID, error), opts ...event.Option) *event.Hook[func(iotago.TransactionID, iotago.BlockID, error)] {
	return m.attachTransactionFailed.Hook(handler, opts...)
}

func (m *MemPool[VoteRank]) OnSignedTransactionAttached(handler func(signedTransactionMetadata mempool.SignedTransactionMetadata), opts ...event.Option) *event.Hook[func(metadata mempool.SignedTransactionMetadata)] {
	return m.signedTransactionAttached.Hook(handler, opts...)
}

func (m *MemPool[VoteRank]) OnTransactionAttached(handler func(transaction mempool.TransactionMetadata), opts ...event.Option) *event.Hook[func(metadata mempool.TransactionMetadata)] {
	return m.transactionAttached.Hook(handler, opts...)
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

// CommitStateDiff commits the state diff for the given slot so that it cannot be modified anymore and returns it.
func (m *MemPool[VoteRank]) CommitStateDiff(slot iotago.SlotIndex) (mempool.StateDiff, error) {
	m.commitmentMutex.Lock()
	defer m.commitmentMutex.Unlock()

	stateDiff, err := m.stateDiff(slot)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to retrieve StateDiff instance for slot %d", slot)
	}

	m.lastCommittedSlot = slot

	return stateDiff, nil
}

func (m *MemPool[VoteRank]) stateDiff(slot iotago.SlotIndex) (*StateDiff, error) {
	if m.lastCommittedSlot >= slot {
		return nil, ierrors.Errorf("slot %d is older than last evicted slot %d", slot, m.lastCommittedSlot)
	}

	kv, err := m.mutationsFunc(slot)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to get state diff for slot %d", slot)
	}

	return lo.Return1(m.stateDiffs.GetOrCreate(slot, func() *StateDiff { return NewStateDiff(slot, kv) })), nil
}

// Reset resets the component to a clean state as if it was created at the last commitment.
func (m *MemPool[VoteRank]) Reset() {
	m.commitmentMutex.Lock()
	defer m.commitmentMutex.Unlock()

	m.stateDiffs.ForEachKey(func(slot iotago.SlotIndex) bool {
		if slot > m.lastCommittedSlot {
			if stateDiff, deleted := m.stateDiffs.DeleteAndReturn(slot); deleted {
				if err := stateDiff.Reset(); err != nil {
					m.errorHandler(ierrors.Wrapf(err, "failed to reset state diff for slot %d", slot))
				}
			}
		}

		return true
	})

	attachmentsToDelete := make([]iotago.SlotIndex, 0)
	m.attachments.ForEach(func(slot iotago.SlotIndex, _ *shrinkingmap.ShrinkingMap[iotago.BlockID, *SignedTransactionMetadata]) {
		if slot > m.lastCommittedSlot {
			attachmentsToDelete = append(attachmentsToDelete, slot)
		}
	})

	for _, slot := range attachmentsToDelete {
		if evictedAttachments := m.attachments.Evict(slot); evictedAttachments != nil {
			evictedAttachments.ForEach(func(id iotago.BlockID, metadata *SignedTransactionMetadata) bool {
				metadata.evictAttachment(id)

				return true
			})
		}
	}

	// We need to clean up all transactions and outputs that were issued beyond the last committed slot.
	m.delayedTransactionEviction.ForEach(func(slot iotago.SlotIndex, delayedTransactions ds.Set[iotago.TransactionID]) bool {
		if slot <= m.lastCommittedSlot {
			return true
		}

		protocolParams := m.apiProvider.APIForSlot(slot).ProtocolParameters()
		genesisSlot := protocolParams.GenesisSlot()
		maxCommittableAge := protocolParams.MaxCommittableAge()

		// No need to evict delayed eviction slot if the committed slot is below maxCommittableAge,
		// as there is nothing to evict anyway at this point.
		if slot <= genesisSlot+maxCommittableAge {
			return true
		}

		delayedEvictionSlot := slot - maxCommittableAge
		delayedTransactions.Range(func(txID iotago.TransactionID) {
			if transaction, exists := m.cachedTransactions.Get(txID); exists {
				transaction.setEvicted()
			}
		})
		m.delayedTransactionEviction.Delete(delayedEvictionSlot)

		return true
	})

	m.delayedOutputStateEviction.ForEach(func(slot iotago.SlotIndex, delayedOutputs *shrinkingmap.ShrinkingMap[iotago.Identifier, *StateMetadata]) bool {
		if slot <= m.lastCommittedSlot {
			return true
		}

		protocolParams := m.apiProvider.APIForSlot(slot).ProtocolParameters()
		genesisSlot := protocolParams.GenesisSlot()
		maxCommittableAge := protocolParams.MaxCommittableAge()

		// No need to evict delayed eviction slot if the committed slot is below maxCommittableAge,
		// as there is nothing to evict anyway at this point.
		if slot <= genesisSlot+maxCommittableAge {
			return true
		}

		delayedEvictionSlot := slot - maxCommittableAge
		delayedOutputs.ForEach(func(stateID iotago.Identifier, state *StateMetadata) bool {
			m.cachedStateRequests.Delete(stateID, state.HasNoSpenders)

			return true
		})
		m.delayedOutputStateEviction.Delete(delayedEvictionSlot)

		return true
	})
}

// Evict evicts the slot with the given slot from the MemPool.
func (m *MemPool[VoteRank]) Evict(slot iotago.SlotIndex) {
	if evictedAttachments := func() *shrinkingmap.ShrinkingMap[iotago.BlockID, *SignedTransactionMetadata] {
		m.commitmentMutex.Lock()
		defer m.commitmentMutex.Unlock()

		m.stateDiffs.Delete(slot)

		return m.attachments.Evict(slot)
	}(); evictedAttachments != nil {
		evictedAttachments.ForEach(func(blockID iotago.BlockID, signedTransactionMetadata *SignedTransactionMetadata) bool {
			signedTransactionMetadata.evictAttachment(blockID)
			return true
		})
	}

	protocolParams := m.apiProvider.APIForSlot(slot).ProtocolParameters()
	genesisSlot := protocolParams.GenesisSlot()
	maxCommittableAge := protocolParams.MaxCommittableAge()

	// No need to evict delayed eviction slot if the committed slot is below maxCommittableAge,
	// as there is nothing to evict anyway at this point.
	if slot <= genesisSlot+maxCommittableAge {
		return
	}

	// See PR #399 for the rationale behind this.
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
			m.cachedStateRequests.Delete(stateID, state.HasNoSpenders)

			return true
		})
		m.delayedOutputStateEviction.Delete(delayedEvictionSlot)
	}
}

func (m *MemPool[VoteRank]) storeTransaction(signedTransaction mempool.SignedTransaction, transaction mempool.Transaction, blockID iotago.BlockID) (storedSignedTransaction *SignedTransactionMetadata, isNewSignedTransaction bool, isNewTransaction bool, err error) {
	m.commitmentMutex.RLock()
	defer m.commitmentMutex.RUnlock()

	if m.lastCommittedSlot >= blockID.Slot() {
		// block will be retained as invalid, we do not store tx failure as it was block's fault
		return nil, false, false, ierrors.Errorf("blockID %s is older than last evicted slot %d", blockID, m.lastCommittedSlot)
	}

	inputReferences, err := m.vm.Inputs(transaction)
	if err != nil {
		return nil, false, false, ierrors.Wrap(err, "failed to get input references of transaction")
	}

	newTransaction := NewTransactionMetadata(transaction, inputReferences)

	storedTransaction, isNewTransaction := m.cachedTransactions.GetOrCreate(newTransaction.ID(), func() *TransactionMetadata { return newTransaction })
	if isNewTransaction {
		m.setupTransaction(storedTransaction)
	}

	newSignedTransaction := NewSignedTransactionMetadata(signedTransaction, storedTransaction)

	storedSignedTransaction, isNewSignedTransaction = m.cachedSignedTransactions.GetOrCreate(signedTransaction.MustID(), func() *SignedTransactionMetadata { return newSignedTransaction })
	if isNewSignedTransaction {
		m.setupSignedTransaction(storedSignedTransaction, storedTransaction)
	}

	storedSignedTransaction.addAttachment(blockID)
	m.attachments.Get(blockID.Slot(), true).Set(blockID, storedSignedTransaction)

	return storedSignedTransaction, isNewSignedTransaction, isNewTransaction, nil
}

func (m *MemPool[VoteRank]) solidifyInputs(transaction *TransactionMetadata) {
	for index, inputReference := range transaction.inputReferences {
		fmt.Println("solidify", inputReference.ReferencedStateID(), "<-", transaction.ID())
		request, created := m.cachedStateRequests.GetOrCreate(inputReference.ReferencedStateID(), func() *promise.Promise[*StateMetadata] {
			return m.requestState(inputReference, true)
		})

		request.OnSuccess(func(inputState *StateMetadata) {
			transaction.publishInput(index, inputState)

			if created {
				m.setupOutputState(inputState)
			}

			if transaction.markInputSolid() {
				fmt.Println("inputs solid of", transaction.ID())
				transaction.executionContext.OnUpdate(func(_ context.Context, executionContext context.Context) {
					fmt.Println("executing", transaction.ID())

					m.executeTransaction(executionContext, transaction)
				})
			}
		})

		request.OnError(func(reason error) {
			// use the sentinel error to mark that the request failed instead of the execution
			transaction.setInvalid(ierrors.Join(mempool.ErrInputSolidificationRequestFailed, reason))
		})
	}
}

func (m *MemPool[VoteRank]) executeTransaction(executionContext context.Context, transaction *TransactionMetadata) {
	if outputStates, err := m.vm.Execute(executionContext, transaction.Transaction()); err != nil {
		transaction.setInvalid(err)
	} else {
		transaction.setExecuted(outputStates)
		fmt.Println("booking", transaction.ID())

		m.bookTransaction(transaction)
	}
}

func (m *MemPool[VoteRank]) bookTransaction(transaction *TransactionMetadata) {
	inputsToFork := lo.Filter(transaction.inputs, func(metadata *StateMetadata) bool {
		return !metadata.state.IsReadOnly()
	})

	m.forkTransaction(transaction, ds.NewSet(lo.Map(inputsToFork, func(stateMetadata *StateMetadata) mempool.StateID {
		return stateMetadata.state.StateID()
	})...))

	// if !lo.Return2(transaction.IsOrphaned()) && transaction.setBooked() {
	if transaction.setBooked() {
		m.publishOutputStates(transaction)
	}
}

func (m *MemPool[VoteRank]) forkTransaction(transactionMetadata *TransactionMetadata, resourceIDs ds.Set[mempool.StateID]) {
	m.spendDAG.CreateSpender(transactionMetadata.ID())

	unsubscribe := transactionMetadata.parentSpenderIDs.OnUpdate(func(appliedMutations ds.SetMutations[iotago.TransactionID]) {
		if err := m.spendDAG.UpdateSpenderParents(transactionMetadata.ID(), appliedMutations.AddedElements(), appliedMutations.DeletedElements()); err != nil {
			panic(err)
		}
	})

	transactionMetadata.OnEvicted(func() {
		unsubscribe()

		m.spendDAG.EvictSpender(transactionMetadata.ID())
	})

	transactionMetadata.spenderIDs.Replace(ds.NewSet(transactionMetadata.id))

	if err := m.spendDAG.UpdateSpentResources(transactionMetadata.ID(), resourceIDs); err != nil {
		// this is a hack, as with a reactive.Variable we cannot set it to 0 and still check if it was orphaned.
		transactionMetadata.orphanedSlot.Set(1)

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
	m.commitmentMutex.RLock()
	defer m.commitmentMutex.RUnlock()

	if m.lastCommittedSlot < blockID.Slot() {
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
		stateDiff, err := m.stateDiff(newIndex)
		if err != nil {
			return ierrors.Wrapf(err, "failed to get state diff for slot %d", newIndex)
		}

		if err = stateDiff.AddTransaction(transaction); err != nil {
			return ierrors.Wrapf(err, "failed to add transaction to state diff, txID: %s", transaction.ID())
		}
	}

	return nil
}

func (m *MemPool[VoteRank]) setup() {
	m.spendDAG.Events().SpenderAccepted.Hook(func(id iotago.TransactionID) {
		if transaction, exists := m.cachedTransactions.Get(id); exists {
			m.commitmentMutex.RLock()
			defer m.commitmentMutex.RUnlock()

			transaction.setConflictAccepted()
		}
	})
}

func (m *MemPool[VoteRank]) setupTransaction(transaction *TransactionMetadata) {
	transaction.OnAccepted(func() {
		// Transactions can only become accepted if there is at least one attachment included.
		if slot := transaction.EarliestIncludedAttachment().Slot(); slot != 0 {
			stateDiff, err := m.stateDiff(slot)
			if err != nil {
				m.errorHandler(ierrors.Wrapf(err, "failed to get state diff for slot %d", slot))

				return
			}

			if err := stateDiff.AddTransaction(transaction); err != nil {
				m.errorHandler(ierrors.Wrapf(err, "failed to add transaction to state diff, txID: %s", transaction.ID()))
			}
		}
	})

	transaction.OnEarliestIncludedAttachmentUpdated(func(prevBlock iotago.BlockID, newBlock iotago.BlockID) {
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

	transaction.OnCommittedSlotUpdated(func(slot iotago.SlotIndex) {
		lo.Return1(m.delayedTransactionEviction.GetOrCreate(slot, func() ds.Set[iotago.TransactionID] { return ds.NewSet[iotago.TransactionID]() })).Add(transaction.ID())
	})

	transaction.OnOrphanedSlotUpdated(func(slot iotago.SlotIndex) {
		lo.Return1(m.delayedTransactionEviction.GetOrCreate(slot, func() ds.Set[iotago.TransactionID] { return ds.NewSet[iotago.TransactionID]() })).Add(transaction.ID())
	})
}

func (m *MemPool[VoteRank]) setupOutputState(stateMetadata *StateMetadata) {
	stateMetadata.onAllSpendersRemoved(func() {
		m.cachedStateRequests.Delete(stateMetadata.state.StateID(), stateMetadata.HasNoSpenders)
	})

	stateMetadata.OnCommittedSlotUpdated(func(slot iotago.SlotIndex) {
		lo.Return1(m.delayedOutputStateEviction.GetOrCreate(slot, func() *shrinkingmap.ShrinkingMap[iotago.Identifier, *StateMetadata] {
			return shrinkingmap.New[iotago.Identifier, *StateMetadata]()
		})).Set(stateMetadata.state.StateID(), stateMetadata)
	})

	stateMetadata.OnOrphanedSlotUpdated(func(slot iotago.SlotIndex) {
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

var _ mempool.MemPool[vote.MockedRank] = new(MemPool[vote.MockedRank])
