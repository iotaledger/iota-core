package mempoolv1

import (
	"context"
	"sync/atomic"

	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	iotago "github.com/iotaledger/iota.go/v4"
)

type TransactionMetadata struct {
	id               iotago.TransactionID
	inputReferences  []mempool.StateReference
	inputs           []*StateMetadata
	outputs          []*StateMetadata
	transaction      mempool.Transaction
	parentSpenderIDs reactive.DerivedSet[iotago.TransactionID]
	spenderIDs       reactive.DerivedSet[iotago.TransactionID]

	// lifecycle events
	unsolidInputsCount uint64
	solid              reactive.Event
	executionContext   reactive.Variable[context.Context]
	executed           reactive.Event
	invalid            reactive.Variable[error]
	booked             reactive.Event
	evicted            reactive.Event

	// predecessors for acceptance
	unacceptedInputsCount uint64
	allInputsAccepted     reactive.Variable[bool]
	conflictAccepted      reactive.Event

	// attachments
	signingTransactions reactive.Set[*SignedTransactionMetadata]

	validAttachments                *shrinkingmap.ShrinkingMap[iotago.BlockID, bool]
	earliestIncludedValidAttachment reactive.Variable[iotago.BlockID]

	// allValidAttachmentsEvicted is set on the slot of the last and newest evicted attachment
	allValidAttachmentsEvicted reactive.Variable[iotago.SlotIndex]

	// mutex needed?
	mutex            syncutils.RWMutex
	attachmentsMutex syncutils.RWMutex

	*inclusionFlags
}

func (t *TransactionMetadata) ValidAttachments() []iotago.BlockID {
	return t.validAttachments.Keys()
}

func NewTransactionMetadata(transaction mempool.Transaction, referencedInputs []mempool.StateReference) *TransactionMetadata {
	transactionID := transaction.MustID()

	return (&TransactionMetadata{
		id:               transactionID,
		inputReferences:  referencedInputs,
		inputs:           make([]*StateMetadata, len(referencedInputs)),
		transaction:      transaction,
		parentSpenderIDs: reactive.NewDerivedSet[iotago.TransactionID](),
		spenderIDs:       reactive.NewDerivedSet[iotago.TransactionID](),

		unsolidInputsCount: uint64(len(referencedInputs)),
		booked:             reactive.NewEvent(),
		solid:              reactive.NewEvent(),
		executionContext:   reactive.NewVariable[context.Context](),
		executed:           reactive.NewEvent(),
		invalid:            reactive.NewVariable[error](),
		evicted:            reactive.NewEvent(),

		unacceptedInputsCount: uint64(len(referencedInputs)),
		allInputsAccepted:     reactive.NewVariable[bool](),
		conflictAccepted:      reactive.NewEvent(),

		signingTransactions: reactive.NewSet[*SignedTransactionMetadata](),

		validAttachments:                shrinkingmap.New[iotago.BlockID, bool](),
		earliestIncludedValidAttachment: reactive.NewVariable[iotago.BlockID](),
		allValidAttachmentsEvicted:      reactive.NewVariable[iotago.SlotIndex](),

		inclusionFlags: newInclusionFlags(),
	}).setup()
}

func (t *TransactionMetadata) ID() iotago.TransactionID {
	return t.id
}

func (t *TransactionMetadata) Transaction() mempool.Transaction {
	return t.transaction
}

func (t *TransactionMetadata) Inputs() ds.Set[mempool.StateMetadata] {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	inputs := ds.NewSet[mempool.StateMetadata]()
	for _, input := range t.inputs {
		inputs.Add(input)
	}

	return inputs
}

func (t *TransactionMetadata) Outputs() ds.Set[mempool.StateMetadata] {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	outputs := ds.NewSet[mempool.StateMetadata]()
	for _, output := range t.outputs {
		outputs.Add(output)
	}

	return outputs
}

func (t *TransactionMetadata) SpenderIDs() reactive.Set[iotago.TransactionID] {
	return t.spenderIDs
}

func (t *TransactionMetadata) publishInput(index int, input *StateMetadata) {
	t.inputs[index] = input

	input.setupSpender(t)
	t.setupInput(input)
}

func (t *TransactionMetadata) setExecuted(outputStates []mempool.State) {
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
	return t.invalid.Get() != nil
}

func (t *TransactionMetadata) OnInvalid(callback func(error)) {
	t.invalid.OnUpdate(func(_ error, newValue error) {
		callback(newValue)
	})
}

func (t *TransactionMetadata) IsBooked() bool {
	return t.booked.WasTriggered()
}

func (t *TransactionMetadata) OnBooked(callback func()) {
	t.booked.OnTrigger(callback)
}

func (t *TransactionMetadata) IsEvicted() bool {
	return t.evicted.WasTriggered()
}

func (t *TransactionMetadata) OnEvicted(callback func()) {
	t.evicted.OnTrigger(callback)
}

func (t *TransactionMetadata) setEvicted() {
	t.evicted.Trigger()
}

func (t *TransactionMetadata) setSolid() bool {
	return t.solid.Trigger()
}

func (t *TransactionMetadata) setBooked() bool {
	return t.booked.Trigger()
}

func (t *TransactionMetadata) setInvalid(reason error) {
	_ = t.invalid.Set(reason)
}

func (t *TransactionMetadata) markInputSolid() (allInputsSolid bool) {
	if atomic.AddUint64(&t.unsolidInputsCount, ^uint64(0)) == 0 {
		return t.setSolid()
	}

	return false
}

func (t *TransactionMetadata) Commit() {
	t.committedSlot.Set(t.earliestIncludedValidAttachment.Get().Slot())
}

func (t *TransactionMetadata) IsConflictAccepted() bool {
	return t.conflictAccepted.WasTriggered()
}

func (t *TransactionMetadata) OnConflictAccepted(callback func()) {
	t.conflictAccepted.OnTrigger(callback)
}

func (t *TransactionMetadata) AllInputsAccepted() bool {
	return t.allInputsAccepted.Get()
}

func (t *TransactionMetadata) setConflictAccepted() {
	if t.conflictAccepted.Trigger() {
		if t.invalid.Get() == nil && t.AllInputsAccepted() && t.EarliestIncludedAttachment().Slot() != 0 {
			t.accepted.Set(true)
		}
	}
}

func (t *TransactionMetadata) setupInput(input *StateMetadata) {
	t.parentSpenderIDs.InheritFrom(input.spenderIDs)

	input.OnRejected(func() { t.rejected.Trigger() })
	input.OnOrphanedSlotUpdated(func(slot iotago.SlotIndex) {
		t.orphanedSlot.Set(slot)
	})
	input.OnAccepted(func() {
		if atomic.AddUint64(&t.unacceptedInputsCount, ^uint64(0)) == 0 {
			if wereAllInputsAccepted := t.allInputsAccepted.Set(true); !wereAllInputsAccepted {
				if t.invalid.Get() == nil && t.IsConflictAccepted() && t.EarliestIncludedAttachment().Slot() != 0 {
					t.accepted.Set(true)
				}
			}
		}
	})

	input.OnAcceptedSpenderUpdated(func(spender mempool.TransactionMetadata) {
		//nolint:forcetypeassert // we can be sure that the spender is a TransactionMetadata
		if spender.(*TransactionMetadata) != nil && spender != t {
			t.rejected.Trigger()
		}
	})

	input.OnSpendCommitted(func(spender mempool.TransactionMetadata) {
		if spender != t {
			spender.OnCommittedSlotUpdated(func(slot iotago.SlotIndex) {
				t.orphanedSlot.Set(slot)
			})
		}
	})
}

func (t *TransactionMetadata) setup() (self *TransactionMetadata) {
	t.allValidAttachmentsEvicted.OnUpdate(func(_ iotago.SlotIndex, slot iotago.SlotIndex) {
		if !lo.Return2(t.CommittedSlot()) {
			t.orphanedSlot.Set(slot)
		}
	})

	t.OnEarliestIncludedAttachmentUpdated(func(previousBlockID iotago.BlockID, newBlockID iotago.BlockID) {
		if previousBlockID.Empty() && !newBlockID.Empty() && t.invalid.Get() == nil && t.IsConflictAccepted() && t.AllInputsAccepted() {
			t.accepted.Set(true)
		}
	})

	return t
}

func (t *TransactionMetadata) addSigningTransaction(signedTransactionMetadata *SignedTransactionMetadata) (added bool) {
	t.attachmentsMutex.Lock()
	defer t.attachmentsMutex.Unlock()

	if added = t.signingTransactions.Add(signedTransactionMetadata); added {
		signedTransactionMetadata.OnEvicted(func() {
			t.signingTransactions.Delete(signedTransactionMetadata)
		})
	}

	return added
}

func (t *TransactionMetadata) markAttachmentIncluded(blockID iotago.BlockID) (included bool) {
	t.attachmentsMutex.Lock()
	defer t.attachmentsMutex.Unlock()

	t.validAttachments.Set(blockID, true)

	if lowestSlotIndex := t.earliestIncludedValidAttachment.Get().Slot(); lowestSlotIndex == 0 || blockID.Slot() < lowestSlotIndex {
		t.earliestIncludedValidAttachment.Set(blockID)
	}

	return true
}

func (t *TransactionMetadata) EarliestIncludedAttachment() iotago.BlockID {
	return t.earliestIncludedValidAttachment.Get()
}

func (t *TransactionMetadata) OnEarliestIncludedAttachmentUpdated(callback func(prevBlock, newBlock iotago.BlockID)) {
	t.earliestIncludedValidAttachment.OnUpdate(callback)
}

func (t *TransactionMetadata) addValidAttachment(blockID iotago.BlockID) (added bool) {
	t.attachmentsMutex.Lock()
	defer t.attachmentsMutex.Unlock()

	return lo.Return2(t.validAttachments.GetOrCreate(blockID, func() bool {
		return false
	}))
}

func (t *TransactionMetadata) evictValidAttachment(id iotago.BlockID) {
	t.attachmentsMutex.Lock()
	defer t.attachmentsMutex.Unlock()

	if t.validAttachments.Delete(id) && t.validAttachments.IsEmpty() {
		t.allValidAttachmentsEvicted.Set(id.Slot())
	}
}
