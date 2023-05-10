package mempoolv1

import (
	"sync/atomic"

	"github.com/iotaledger/iota-core/pkg/core/promise"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
)

type TransactionInclusion struct {
	unacceptedInputsCount uint64
	allInputsAccepted     *promise.Event

	*InclusionState
}

func NewTransactionInclusion(inputCount int) *TransactionInclusion {
	return &TransactionInclusion{
		unacceptedInputsCount: uint64(inputCount),
		allInputsAccepted:     promise.NewEvent(),
		InclusionState:        NewInclusionState(),
	}
}

func (t *TransactionInclusion) Commit() {
	t.setCommitted()
}

func (t *TransactionInclusion) AllInputsAccepted() bool {
	return t.allInputsAccepted.WasTriggered()
}

func (t *TransactionInclusion) OnAllInputsAccepted(callback func()) {
	t.allInputsAccepted.OnTrigger(callback)
}

func (t *TransactionInclusion) dependsOnInput(input *StateWithMetadata) {
	input.OnRejected(t.setRejected)
	input.OnOrphaned(t.setOrphaned)

	input.OnAccepted(func() {
		if atomic.AddUint64(&t.unacceptedInputsCount, ^uint64(0)) == 0 {
			t.allInputsAccepted.Trigger()
		}
	})

	input.OnSpendAccepted(func(spender mempool.TransactionWithMetadata) {
		if spender.(*TransactionMetadata).TransactionInclusion != t {
			t.setRejected()
		}
	})

	input.OnSpendCommitted(func(spender mempool.TransactionWithMetadata) {
		if spender.(*TransactionMetadata).TransactionInclusion != t {
			t.setOrphaned()
		}
	})
}

func (t *TransactionInclusion) dependsOnAttachments(attachments *Attachments) {
	attachments.OnAllAttachmentsEvicted(func() {
		if !t.IsCommitted() {
			t.setOrphaned()
		}
	})
}
