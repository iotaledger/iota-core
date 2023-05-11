package mempoolv1

import (
	"sync"
	"sync/atomic"

	"golang.org/x/xerrors"

	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/core/promise"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledger"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	iotago "github.com/iotaledger/iota.go/v4"
)

type TransactionMetadata struct {
	id              iotago.TransactionID
	inputReferences []ledger.StateReference
	inputs          []*StateMetadata
	outputs         []*StateMetadata
	transaction     mempool.Transaction
	conflictIDs     *advancedset.AdvancedSet[iotago.TransactionID]

	// lifecycle events
	unsolidInputsCount uint64
	stored             *promise.Event
	solid              *promise.Event
	executed           *promise.Event
	invalid            *promise.Event1[error]
	booked             *promise.Event

	// predecessors for acceptance
	unacceptedInputsCount uint64
	allInputsAccepted     *promise.Event
	conflicting           *promise.Event
	conflictAccepted      *promise.Event

	// attachments
	attachments           *shrinkingmap.ShrinkingMap[iotago.BlockID, bool]
	earliestIncludedSlot  *promise.Value[iotago.SlotIndex]
	allAttachmentsEvicted *promise.Event

	// mutex needed?
	mutex sync.RWMutex

	attachmentsMutex sync.RWMutex

	*Inclusion
}

func NewTransactionWithMetadata(transaction mempool.Transaction) (*TransactionMetadata, error) {
	transactionID, transactionIDErr := transaction.ID()
	if transactionIDErr != nil {
		return nil, xerrors.Errorf("failed to retrieve transaction ID: %w", transactionIDErr)
	}

	inputReferences, inputsErr := transaction.Inputs()
	if inputsErr != nil {
		return nil, xerrors.Errorf("failed to retrieve inputReferences of transaction %s: %w", transactionID, inputsErr)
	}

	t := &TransactionMetadata{
		id:              transactionID,
		inputReferences: inputReferences,
		inputs:          make([]*StateMetadata, len(inputReferences)),
		transaction:     transaction,
		conflictIDs:     advancedset.New[iotago.TransactionID](),

		unsolidInputsCount: uint64(len(inputReferences)),
		stored:             promise.NewEvent(),
		booked:             promise.NewEvent(),
		solid:              promise.NewEvent(),
		executed:           promise.NewEvent(),
		invalid:            promise.NewEvent1[error](),

		unacceptedInputsCount: uint64(len(inputReferences)),
		allInputsAccepted:     promise.NewEvent(),
		conflicting:           promise.NewEvent(),
		conflictAccepted:      promise.NewEvent(),

		attachments:           shrinkingmap.New[iotago.BlockID, bool](),
		earliestIncludedSlot:  promise.NewValue[iotago.SlotIndex](),
		allAttachmentsEvicted: promise.NewEvent(),

		Inclusion: NewInclusion(),
	}

	t.setupBehavior()

	return t, nil
}

func (t *TransactionMetadata) ID() iotago.TransactionID {
	return t.id
}

func (t *TransactionMetadata) Transaction() mempool.Transaction {
	return t.transaction
}

func (t *TransactionMetadata) Inputs() *advancedset.AdvancedSet[mempool.StateMetadata] {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	inputs := advancedset.New[mempool.StateMetadata]()
	for _, input := range t.inputs {
		inputs.Add(input)
	}

	return inputs
}

func (t *TransactionMetadata) Outputs() *advancedset.AdvancedSet[mempool.StateMetadata] {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	outputs := advancedset.New[mempool.StateMetadata]()
	for _, output := range t.outputs {
		outputs.Add(output)
	}

	return outputs
}

func (t *TransactionMetadata) publishInput(index int, input *StateMetadata) (allInputsSolid bool) {
	t.inputs[index] = input

	input.StateLifecycle.dependsOnSpender(t)
	t.setupInputDependencies(input)

	return t.markInputSolid()
}

func (t *TransactionMetadata) setExecuted(outputStates []ledger.State) {
	t.mutex.Lock()
	for _, outputState := range outputStates {
		t.outputs = append(t.outputs, NewStateMetadata(outputState, t))
	}
	t.mutex.Unlock()

	t.executed.Trigger()
}

func (t *TransactionMetadata) IsStored() bool {
	return t.stored.WasTriggered()
}

func (t *TransactionMetadata) OnStored(callback func()) {
	t.stored.OnTrigger(callback)
}

func (t *TransactionMetadata) IsSolid() bool {
	return t.solid.WasTriggered()
}

func (t *TransactionMetadata) OnSolid(callback func()) {
	t.solid.OnTrigger(callback)
}

func (t *TransactionMetadata) IsExecuted() bool {
	return t.executed.WasTriggered()
}

func (t *TransactionMetadata) OnExecuted(callback func()) {
	t.executed.OnTrigger(callback)
}

func (t *TransactionMetadata) IsInvalid() bool {
	return t.invalid.WasTriggered()
}

func (t *TransactionMetadata) OnInvalid(callback func(error)) {
	t.invalid.OnTrigger(callback)
}

func (t *TransactionMetadata) IsBooked() bool {
	return t.booked.WasTriggered()
}

func (t *TransactionMetadata) OnBooked(callback func()) {
	t.booked.OnTrigger(callback)
}

func (t *TransactionMetadata) setStored() bool {
	return t.stored.Trigger()
}

func (t *TransactionMetadata) setSolid() bool {
	return t.solid.Trigger()
}

func (t *TransactionMetadata) setBooked() bool {
	return t.booked.Trigger()
}

func (t *TransactionMetadata) setInvalid(reason error) {
	t.invalid.Trigger(reason)
}

func (t *TransactionMetadata) markInputSolid() (allInputsSolid bool) {
	if atomic.AddUint64(&t.unsolidInputsCount, ^uint64(0)) == 0 {
		return t.setSolid()
	}

	return false
}

func (t *TransactionMetadata) Commit() {
	t.setCommitted()
}

func (t *TransactionMetadata) IsConflicting() bool {
	return t.conflicting.WasTriggered()
}

func (t *TransactionMetadata) OnConflicting(callback func()) {
	t.conflicting.OnTrigger(callback)
}

func (t *TransactionMetadata) IsConflictAccepted() bool {
	return !t.IsConflicting() || t.conflictAccepted.WasTriggered()
}

func (t *TransactionMetadata) OnConflictAccepted(callback func()) {
	t.conflictAccepted.OnTrigger(callback)
}

func (t *TransactionMetadata) IsIncluded() bool {
	return t.earliestIncludedSlot.Get() != 0
}

func (t *TransactionMetadata) AllInputsAccepted() bool {
	return t.allInputsAccepted.WasTriggered()
}

func (t *TransactionMetadata) OnAllInputsAccepted(callback func()) {
	t.allInputsAccepted.OnTrigger(callback)
}

func (t *TransactionMetadata) setConflicting() {
	t.conflicting.Trigger()
}

func (t *TransactionMetadata) setConflictAccepted() {
	if t.conflictAccepted.Trigger() {
		if t.AllInputsAccepted() && t.IsIncluded() {
			t.setAccepted()
		}
	}
}

func (t *TransactionMetadata) setupInputDependencies(input *StateMetadata) {
	input.OnRejected(t.setRejected)
	input.OnOrphaned(t.setOrphaned)

	input.OnAccepted(func() {
		if atomic.AddUint64(&t.unacceptedInputsCount, ^uint64(0)) == 0 {
			if t.allInputsAccepted.Trigger() {
				if t.IsConflictAccepted() && t.IsIncluded() {
					t.setAccepted()
				}
			}
		}
	})

	input.OnSpendAccepted(func(spender mempool.TransactionMetadata) {
		if spender != t {
			t.setRejected()
		}
	})

	input.OnSpendCommitted(func(spender mempool.TransactionMetadata) {
		if spender != t {
			t.setOrphaned()
		}
	})
}

func (t *TransactionMetadata) setupBehavior() (self *TransactionMetadata) {
	t.OnAllAttachmentsEvicted(func() {
		if !t.IsCommitted() {
			t.setOrphaned()
		}
	})

	t.OnEarliestIncludedSlotUpdated(func(previousIndex, newIndex iotago.SlotIndex) {
		if isIncluded, wasIncluded := newIndex != 0, previousIndex != 0; isIncluded != wasIncluded {
			if !isIncluded {
				t.setPending()
			} else if t.AllInputsAccepted() && t.IsConflictAccepted() {
				t.setAccepted()
			}
		}
	})

	return t
}

func (t *TransactionMetadata) Add(blockID iotago.BlockID) (added bool) {
	t.attachmentsMutex.Lock()
	defer t.attachmentsMutex.Unlock()

	return lo.Return2(t.attachments.GetOrCreate(blockID, func() bool { return false }))
}

func (t *TransactionMetadata) MarkIncluded(blockID iotago.BlockID) (included bool) {
	t.attachmentsMutex.Lock()
	defer t.attachmentsMutex.Unlock()

	t.attachments.Set(blockID, true)

	if lowestSlotIndex := t.earliestIncludedSlot.Get(); lowestSlotIndex == 0 || blockID.Index() < lowestSlotIndex {
		t.earliestIncludedSlot.Set(blockID.Index())
	}

	return true
}

func (t *TransactionMetadata) MarkOrphaned(blockID iotago.BlockID) (orphaned bool) {
	t.attachmentsMutex.Lock()
	defer t.attachmentsMutex.Unlock()

	previousState, exists := t.attachments.Get(blockID)
	if !exists {
		return false
	}

	t.evict(blockID)

	if previousState && blockID.Index() == t.earliestIncludedSlot.Get() {
		t.earliestIncludedSlot.Set(t.findLowestIncludedSlotIndex())
	}

	return true
}

func (t *TransactionMetadata) EarliestIncludedSlot() iotago.SlotIndex {
	return t.earliestIncludedSlot.Get()
}

func (t *TransactionMetadata) OnEarliestIncludedSlotUpdated(callback func(prevIndex, newIndex iotago.SlotIndex)) (unsubscribe func()) {
	return t.earliestIncludedSlot.OnUpdate(callback)
}

func (t *TransactionMetadata) OnAllAttachmentsEvicted(callback func()) {
	t.allAttachmentsEvicted.OnTrigger(callback)
}

func (t *TransactionMetadata) Evict(id iotago.BlockID) {
	t.attachmentsMutex.Lock()
	defer t.attachmentsMutex.Unlock()

	t.evict(id)
}

func (t *TransactionMetadata) evict(id iotago.BlockID) {
	if t.attachments.Delete(id) && t.attachments.IsEmpty() {
		t.allAttachmentsEvicted.Trigger()
	}
}

func (t *TransactionMetadata) findLowestIncludedSlotIndex() iotago.SlotIndex {
	var lowestIncludedSlotIndex iotago.SlotIndex

	t.attachments.ForEach(func(blockID iotago.BlockID, included bool) bool {
		if included && (lowestIncludedSlotIndex == 0 || blockID.Index() < lowestIncludedSlotIndex) {
			lowestIncludedSlotIndex = blockID.Index()
		}

		return true
	})

	return lowestIncludedSlotIndex
}
