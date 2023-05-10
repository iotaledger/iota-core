package mempoolv1

import (
	"sync/atomic"

	"github.com/iotaledger/iota-core/pkg/core/promise"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	iotago "github.com/iotaledger/iota.go/v4"
)

type TransactionInclusion struct {
	unacceptedInputsCount uint64
	allInputsAccepted     *promise.Event
	conflicting           *promise.Event
	conflictAccepted      *promise.Event
	included              *promise.Value[bool]

	*Inclusion
}

func NewTransactionInclusion(inputCount int) *TransactionInclusion {
	t := &TransactionInclusion{
		unacceptedInputsCount: uint64(inputCount),
		allInputsAccepted:     promise.NewEvent(),
		conflicting:           promise.NewEvent(),
		conflictAccepted:      promise.NewEvent(),
		included:              promise.NewValue[bool](),
		Inclusion:             NewInclusion(),
	}

	t.OnConflictAccepted(func() {
		if t.AllInputsAccepted() && t.IsIncluded() {
			t.setAccepted()
		}
	})

	t.OnAllInputsAccepted(func() {
		if t.IsConflictAccepted() && t.IsIncluded() {
			t.setAccepted()
		}
	})

	t.OnIncludedUpdated(func(_, isIncluded bool) {
		if !isIncluded {
			t.setPending()
		} else if t.AllInputsAccepted() && t.IsConflictAccepted() {
			t.setAccepted()
		}
	})

	return t
}

func (t *TransactionInclusion) Commit() {
	t.setCommitted()
}

func (t *TransactionInclusion) IsConflicting() bool {
	return t.conflicting.WasTriggered()
}

func (t *TransactionInclusion) OnConflicting(callback func()) {
	t.conflicting.OnTrigger(callback)
}

func (t *TransactionInclusion) IsConflictAccepted() bool {
	return !t.IsConflicting() || t.conflictAccepted.WasTriggered()
}

func (t *TransactionInclusion) OnConflictAccepted(callback func()) {
	t.conflictAccepted.OnTrigger(callback)
}

func (t *TransactionInclusion) IsIncluded() bool {
	return t.included.Get()
}

func (t *TransactionInclusion) OnIncludedUpdated(callback func(previousValue, newValue bool)) {
	t.included.OnUpdate(callback)
}

func (t *TransactionInclusion) AllInputsAccepted() bool {
	return t.allInputsAccepted.WasTriggered()
}

func (t *TransactionInclusion) OnAllInputsAccepted(callback func()) {
	t.allInputsAccepted.OnTrigger(callback)
}

func (t *TransactionInclusion) setConflicting() {
	t.conflicting.Trigger()
}

func (t *TransactionInclusion) setConflictAccepted() {
	t.conflictAccepted.Trigger()
}

func (t *TransactionInclusion) decreaseUnacceptedInputsCount() {
	if atomic.AddUint64(&t.unacceptedInputsCount, ^uint64(0)) == 0 {
		t.allInputsAccepted.Trigger()
	}
}

func (t *TransactionInclusion) rejectIfLosingSpend(spender mempool.TransactionMetadata) {
	if spender.(*TransactionMetadata).TransactionInclusion != t {
		t.setRejected()
	}
}

func (t *TransactionInclusion) orphanIfLosingSpend(spender mempool.TransactionMetadata) {
	if spender.(*TransactionMetadata).TransactionInclusion != t {
		t.setOrphaned()
	}
}

func (t *TransactionInclusion) dependsOnInput(input *StateMetadata) {
	input.OnRejected(t.setRejected)
	input.OnOrphaned(t.setOrphaned)
	input.OnAccepted(t.decreaseUnacceptedInputsCount)
	input.OnSpendAccepted(t.rejectIfLosingSpend)
	input.OnSpendCommitted(t.orphanIfLosingSpend)
}

func (t *TransactionInclusion) dependsOnAttachments(attachments *Attachments) {
	attachments.OnAllAttachmentsEvicted(func() {
		if !t.IsCommitted() {
			t.setOrphaned()
		}
	})

	attachments.OnEarliestIncludedSlotUpdated(func(_, newIndex iotago.SlotIndex) {
		t.included.Set(newIndex != 0)
	})
}
