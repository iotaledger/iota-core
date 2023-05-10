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
	input.inclusionState.OnRejected(t.setRejected)
	input.inclusionState.OnOrphaned(t.setOrphaned)

	input.inclusionState.OnAccepted(func() {
		if atomic.AddUint64(&t.unacceptedInputsCount, ^uint64(0)) == 0 {
			t.allInputsAccepted.Trigger()
		}
	})

	input.spentState.OnSpendAccepted(func(spender mempool.TransactionWithMetadata) {
		if spender.Inclusion() != t {
			t.setRejected()
		}
	})

	input.spentState.OnSpendCommitted(func(spender mempool.TransactionWithMetadata) {
		if spender.Inclusion() != t {
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
