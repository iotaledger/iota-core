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
	return &TransactionInclusion{
		unacceptedInputsCount: uint64(inputCount),
		allInputsAccepted:     promise.NewEvent(),
		conflicting:           promise.NewEvent(),
		conflictAccepted:      promise.NewEvent(),
		included:              promise.NewValue[bool](),
		Inclusion:             NewInclusion(),
	}
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
	if t.conflictAccepted.Trigger() {
		if t.AllInputsAccepted() && t.IsIncluded() {
			t.setAccepted()
		}
	}
}

func (t *TransactionInclusion) setupInputDependencies(input *StateMetadata) {
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
		if spender.(*TransactionMetadata).TransactionInclusion != t {
			t.setRejected()
		}
	})

	input.OnSpendCommitted(func(spender mempool.TransactionMetadata) {
		if spender.(*TransactionMetadata).TransactionInclusion != t {
			t.setOrphaned()
		}
	})
}

func (t *TransactionInclusion) setupAttachmentsDependencies(attachments *Attachments) {
	attachments.OnAllAttachmentsEvicted(func() {
		if !t.IsCommitted() {
			t.setOrphaned()
		}
	})

	attachments.OnEarliestIncludedSlotUpdated(func(_, newIndex iotago.SlotIndex) {
		if isIncluded := newIndex != 0; isIncluded != t.included.Set(newIndex != 0) {
			if !isIncluded {
				t.setPending()
			} else if t.AllInputsAccepted() && t.IsConflictAccepted() {
				t.setAccepted()
			}
		}
	})
}
