package mempoolv1

import (
	"sync/atomic"

	"github.com/iotaledger/iota-core/pkg/core/promise"
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

func (t *TransactionInclusion) markInputAccepted() {
	if atomic.AddUint64(&t.unacceptedInputsCount, ^uint64(0)) == 0 {
		t.allInputsAccepted.Trigger()
	}
}

type InclusionState struct {
	accepted  *promise.Event
	committed *promise.Event
	rejected  *promise.Event
	orphaned  *promise.Event
}

func NewInclusionState() *InclusionState {
	return &InclusionState{
		accepted:  promise.NewEvent(),
		committed: promise.NewEvent(),
		rejected:  promise.NewEvent(),
		orphaned:  promise.NewEvent(),
	}
}

func (s *InclusionState) IsAccepted() bool {
	return s.accepted.WasTriggered()
}

func (s *InclusionState) OnAccepted(callback func()) {
	s.accepted.OnTrigger(callback)
}

func (s *InclusionState) IsCommitted() bool {
	return s.committed.WasTriggered()
}

func (s *InclusionState) OnCommitted(callback func()) {
	s.committed.OnTrigger(callback)
}

func (s *InclusionState) setCommitted() {
	s.committed.Trigger()
}

func (s *InclusionState) IsRejected() bool {
	return s.rejected.WasTriggered()
}

func (s *InclusionState) OnRejected(callback func()) {
	s.rejected.OnTrigger(callback)
}

func (s *InclusionState) IsOrphaned() bool {
	return s.orphaned.WasTriggered()
}

func (s *InclusionState) OnOrphaned(callback func()) {
	s.orphaned.OnTrigger(callback)
}

func (s *InclusionState) setAccepted() {
	s.accepted.Trigger()
}

func (s *InclusionState) setRejected() {
	s.rejected.Trigger()
}

func (s *InclusionState) setOrphaned() {
	s.orphaned.Trigger()
}

func (s *InclusionState) inheritFrom(source *InclusionState) {
	source.OnAccepted(s.setAccepted)
	source.OnRejected(s.setRejected)
	source.OnCommitted(s.setCommitted)
	source.OnOrphaned(s.setOrphaned)
}
