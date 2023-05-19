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
	id                iotago.TransactionID
	inputReferences   []ledger.StateReference
	inputs            []*StateMetadata
	outputs           []*StateMetadata
	transaction       mempool.Transaction
	parentConflictIDs *promise.Set[iotago.TransactionID]
	conflictIDs       *promise.Set[iotago.TransactionID]

	// lifecycle events
	unsolidInputsCount uint64
	solid              *promise.Event
	executed           *promise.Event
	invalid            *promise.Event1[error]
	booked             *promise.Event

	// predecessors for acceptance
	unacceptedInputsCount uint64
	allInputsAccepted     *promise.Value[bool]
	conflicting           *promise.Event
	conflictAccepted      *promise.Event

	// attachments
	attachments                *shrinkingmap.ShrinkingMap[iotago.BlockID, bool]
	earliestIncludedAttachment *promise.Value[iotago.BlockID]
	allAttachmentsEvicted      *promise.Event

	// mutex needed?
	mutex sync.RWMutex

	attachmentsMutex sync.RWMutex

	*inclusionFlags
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

	return (&TransactionMetadata{
		id:                transactionID,
		inputReferences:   inputReferences,
		inputs:            make([]*StateMetadata, len(inputReferences)),
		transaction:       transaction,
		parentConflictIDs: promise.NewSet[iotago.TransactionID](),
		conflictIDs:       promise.NewSet[iotago.TransactionID](),

		unsolidInputsCount: uint64(len(inputReferences)),
		booked:             promise.NewEvent(),
		solid:              promise.NewEvent(),
		executed:           promise.NewEvent(),
		invalid:            promise.NewEvent1[error](),

		unacceptedInputsCount: uint64(len(inputReferences)),
		allInputsAccepted:     promise.NewValue[bool](),
		conflicting:           promise.NewEvent(),
		conflictAccepted:      promise.NewEvent(),

		attachments:                shrinkingmap.New[iotago.BlockID, bool](),
		earliestIncludedAttachment: promise.NewValue[iotago.BlockID](),
		allAttachmentsEvicted:      promise.NewEvent(),

		inclusionFlags: newInclusionFlags(),
	}).setup(), nil
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

func (t *TransactionMetadata) ConflictIDs() *promise.Set[iotago.TransactionID] {
	return t.conflictIDs
}

func (t *TransactionMetadata) publishInputAndCheckSolidity(index int, input *StateMetadata) (allInputsSolid bool) {
	t.inputs[index] = input

	input.setupSpender(t)
	t.setupInput(input)

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

func (t *TransactionMetadata) AllInputsAccepted() bool {
	return t.allInputsAccepted.Get()
}

func (t *TransactionMetadata) setConflicting() {
	t.conflicting.Trigger()
}

func (t *TransactionMetadata) setConflictAccepted() {
	if t.conflictAccepted.Trigger() {
		if t.AllInputsAccepted() && t.EarliestIncludedAttachment().Index() != 0 {
			t.setAccepted()
		}
	}
}

func (t *TransactionMetadata) setupInput(input *StateMetadata) {
	t.parentConflictIDs.InheritFrom(input.conflictIDs)

	input.OnRejected(t.setRejected)
	input.OnOrphaned(t.setOrphaned)

	input.OnAccepted(func() {
		if atomic.AddUint64(&t.unacceptedInputsCount, ^uint64(0)) == 0 {
			if wereAllInputsAccepted := t.allInputsAccepted.Set(true); !wereAllInputsAccepted {
				if t.IsConflictAccepted() && t.EarliestIncludedAttachment().Index() != 0 {
					t.setAccepted()
				}
			}
		}
	})

	input.OnPending(func() {
		if atomic.AddUint64(&t.unacceptedInputsCount, 1) == 1 && t.allInputsAccepted.Set(false) {
			t.setPending()
		}
	})

	input.OnAcceptedSpenderUpdated(func(spender mempool.TransactionMetadata) {
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

func (t *TransactionMetadata) setup() (self *TransactionMetadata) {
	cancelConflictInheritance := t.conflictIDs.InheritFrom(t.parentConflictIDs)

	t.OnConflicting(func() {
		cancelConflictInheritance()

		t.conflictIDs.Set(advancedset.New[iotago.TransactionID](t.id))
	})

	t.allAttachmentsEvicted.OnTrigger(func() {
		if !t.IsCommitted() {
			t.setOrphaned()
		}
	})

	t.OnEarliestIncludedAttachmentUpdated(func(previousIndex, newIndex iotago.BlockID) {
		if isIncluded, wasIncluded := newIndex.Index() != 0, previousIndex.Index() != 0; isIncluded != wasIncluded {
			if !isIncluded {
				t.setPending()
			} else if t.AllInputsAccepted() && t.IsConflictAccepted() {
				t.setAccepted()
			}
		}
	})

	return t
}

func (t *TransactionMetadata) addAttachment(blockID iotago.BlockID) (added bool) {
	t.attachmentsMutex.Lock()
	defer t.attachmentsMutex.Unlock()

	return lo.Return2(t.attachments.GetOrCreate(blockID, func() bool { return false }))
}

func (t *TransactionMetadata) markAttachmentIncluded(blockID iotago.BlockID) (included bool) {
	t.attachmentsMutex.Lock()
	defer t.attachmentsMutex.Unlock()

	t.attachments.Set(blockID, true)

	if lowestSlotIndex := t.earliestIncludedAttachment.Get().Index(); lowestSlotIndex == 0 || blockID.Index() < lowestSlotIndex {
		t.earliestIncludedAttachment.Set(blockID)
	}

	return true
}

func (t *TransactionMetadata) markAttachmentOrphaned(blockID iotago.BlockID) (orphaned bool) {
	t.attachmentsMutex.Lock()
	defer t.attachmentsMutex.Unlock()

	previousState, exists := t.attachments.Get(blockID)
	if !exists {
		return false
	}

	t.evictAttachment(blockID)

	if previousState && blockID == t.earliestIncludedAttachment.Get() {
		t.earliestIncludedAttachment.Set(t.findLowestIncludedAttachment())
	}

	return true
}

func (t *TransactionMetadata) EarliestIncludedAttachment() iotago.BlockID {
	return t.earliestIncludedAttachment.Get()
}

func (t *TransactionMetadata) OnEarliestIncludedAttachmentUpdated(callback func(prevBlock, newBlock iotago.BlockID)) {
	t.earliestIncludedAttachment.OnUpdate(callback)
}

func (t *TransactionMetadata) evictAttachment(id iotago.BlockID) {
	if t.attachments.Delete(id) && t.attachments.IsEmpty() {
		t.allAttachmentsEvicted.Trigger()
	}
}

func (t *TransactionMetadata) findLowestIncludedAttachment() iotago.BlockID {
	var lowestIncludedBlock iotago.BlockID

	t.attachments.ForEach(func(blockID iotago.BlockID, included bool) bool {
		if included && (lowestIncludedBlock.Index() == 0 || blockID.Index() < lowestIncludedBlock.Index()) {
			lowestIncludedBlock = blockID
		}

		return true
	})

	return lowestIncludedBlock
}
