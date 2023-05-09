package mempoolv1

import "github.com/iotaledger/iota-core/pkg/core/promise"

type LifecycleState struct {
	stored   *promise.Event
	solid    *promise.Event
	executed *promise.Event
	invalid  *promise.Event1[error]
	booked   *promise.Event
}

func NewLifecycleState() *LifecycleState {
	return &LifecycleState{
		stored:   promise.NewEvent(),
		booked:   promise.NewEvent(),
		solid:    promise.NewEvent(),
		executed: promise.NewEvent(),
		invalid:  promise.NewEvent1[error](),
	}
}

func (t *LifecycleState) IsStored() bool {
	return t.stored.WasTriggered()
}

func (t *LifecycleState) OnStored(callback func()) {
	t.stored.OnTrigger(callback)
}

func (t *LifecycleState) IsSolid() bool {
	return t.solid.WasTriggered()
}

func (t *LifecycleState) OnSolid(callback func()) {
	t.solid.OnTrigger(callback)
}

func (t *LifecycleState) IsExecuted() bool {
	return t.executed.WasTriggered()
}

func (t *LifecycleState) OnExecuted(callback func()) {
	t.executed.OnTrigger(callback)
}

func (t *LifecycleState) IsInvalid() bool {
	return t.invalid.WasTriggered()
}

func (t *LifecycleState) OnInvalid(callback func(error)) {
	t.invalid.OnTrigger(callback)
}

func (t *LifecycleState) IsBooked() bool {
	return t.booked.WasTriggered()
}

func (t *LifecycleState) OnBooked(callback func()) {
	t.booked.OnTrigger(callback)
}

func (t *LifecycleState) setStored() {
	t.stored.Trigger()
}

func (t *LifecycleState) setSolid() {
	t.solid.Trigger()
}

func (t *LifecycleState) setBooked() {
	t.booked.Trigger()
}

func (t *LifecycleState) setInvalid(reason error) {
	t.invalid.Trigger(reason)
}
