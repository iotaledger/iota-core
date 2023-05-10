package mempoolv1

import (
	"github.com/iotaledger/iota-core/pkg/core/promise"
)

// Inclusion represents important flags and events that relate to the inclusion of an entity in the distributed ledger.
type Inclusion struct {
	accepted  *promise.Event
	committed *promise.Event
	rejected  *promise.Event
	orphaned  *promise.Event
}

// NewInclusion creates a new Inclusion instance.
func NewInclusion() *Inclusion {
	return &Inclusion{
		accepted:  promise.NewEvent(),
		committed: promise.NewEvent(),
		rejected:  promise.NewEvent(),
		orphaned:  promise.NewEvent(),
	}
}

// IsAccepted returns true if the entity was accepted.
func (s *Inclusion) IsAccepted() bool {
	return s.accepted.WasTriggered()
}

// OnAccepted registers a callback that gets triggered when the entity gets accepted.
func (s *Inclusion) OnAccepted(callback func()) {
	s.accepted.OnTrigger(callback)
}

// IsRejected returns true if the entity was rejected.
func (s *Inclusion) IsRejected() bool {
	return s.rejected.WasTriggered()
}

// OnRejected registers a callback that gets triggered when the entity gets rejected.
func (s *Inclusion) OnRejected(callback func()) {
	s.rejected.OnTrigger(callback)
}

// IsCommitted returns true if the entity was committed.
func (s *Inclusion) IsCommitted() bool {
	return s.committed.WasTriggered()
}

// OnCommitted registers a callback that gets triggered when the entity gets committed.
func (s *Inclusion) OnCommitted(callback func()) {
	s.committed.OnTrigger(callback)
}

// IsOrphaned returns true if the entity was orphaned.
func (s *Inclusion) IsOrphaned() bool {
	return s.orphaned.WasTriggered()
}

// OnOrphaned registers a callback that gets triggered when the entity gets orphaned.
func (s *Inclusion) OnOrphaned(callback func()) {
	s.orphaned.OnTrigger(callback)
}

// setAccepted marks the entity as accepted.
func (s *Inclusion) setAccepted() {
	s.accepted.Trigger()
}

// setRejected marks the entity as rejected.
func (s *Inclusion) setRejected() {
	s.rejected.Trigger()
}

// setCommitted marks the entity as committed.
func (s *Inclusion) setCommitted() {
	s.committed.Trigger()
}

// setOrphaned marks the entity as orphaned.
func (s *Inclusion) setOrphaned() {
	s.orphaned.Trigger()
}
