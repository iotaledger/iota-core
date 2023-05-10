package mempoolv1

import (
	"sync/atomic"

	"github.com/iotaledger/iota-core/pkg/core/promise"
)

type TransactionLifecycle struct {
	unsolidInputsCount uint64
	stored             *promise.Event
	solid              *promise.Event
	executed           *promise.Event
	invalid            *promise.Event1[error]
	booked             *promise.Event
}

func NewTransactionLifecycle(inputsCount int) *TransactionLifecycle {
	return &TransactionLifecycle{
		unsolidInputsCount: uint64(inputsCount),
		stored:             promise.NewEvent(),
		booked:             promise.NewEvent(),
		solid:              promise.NewEvent(),
		executed:           promise.NewEvent(),
		invalid:            promise.NewEvent1[error](),
	}
}

func (t *TransactionLifecycle) IsStored() bool {
	return t.stored.WasTriggered()
}

func (t *TransactionLifecycle) OnStored(callback func()) {
	t.stored.OnTrigger(callback)
}

func (t *TransactionLifecycle) IsSolid() bool {
	return t.solid.WasTriggered()
}

func (t *TransactionLifecycle) OnSolid(callback func()) {
	t.solid.OnTrigger(callback)
}

func (t *TransactionLifecycle) IsExecuted() bool {
	return t.executed.WasTriggered()
}

func (t *TransactionLifecycle) OnExecuted(callback func()) {
	t.executed.OnTrigger(callback)
}

func (t *TransactionLifecycle) IsInvalid() bool {
	return t.invalid.WasTriggered()
}

func (t *TransactionLifecycle) OnInvalid(callback func(error)) {
	t.invalid.OnTrigger(callback)
}

func (t *TransactionLifecycle) IsBooked() bool {
	return t.booked.WasTriggered()
}

func (t *TransactionLifecycle) OnBooked(callback func()) {
	t.booked.OnTrigger(callback)
}

func (t *TransactionLifecycle) setStored() bool {
	return t.stored.Trigger()
}

func (t *TransactionLifecycle) setSolid() bool {
	return t.solid.Trigger()
}

func (t *TransactionLifecycle) setBooked() {
	t.booked.Trigger()
}

func (t *TransactionLifecycle) setInvalid(reason error) {
	t.invalid.Trigger(reason)
}

func (t *TransactionLifecycle) markInputSolid() (allInputsSolid bool) {
	if atomic.AddUint64(&t.unsolidInputsCount, ^uint64(0)) == 0 {
		return t.setSolid()
	}

	return false
}
