package mempoolv1

import (
	"sync/atomic"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/core/promise"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledger"
	iotago "github.com/iotaledger/iota.go/v4"
)

type StateWithMetadata struct {
	id                iotago.OutputID
	sourceTransaction *TransactionWithMetadata
	consumerCount     uint64
	state             ledger.State

	accepted  *promise.Event
	committed *promise.Event
	rejected  *promise.Event
	evicted   *promise.Event

	spent          *promise.Event
	doubleSpent    *promise.Event
	spendAccepted  *promise.Event1[*TransactionWithMetadata]
	spendCommitted *promise.Event1[*TransactionWithMetadata]

	allConsumersEvicted *promise.Event
}

func NewStateWithMetadata(state ledger.State, optSource ...*TransactionWithMetadata) *StateWithMetadata {
	return &StateWithMetadata{
		id:                state.ID(),
		sourceTransaction: lo.First(optSource),
		state:             state,

		accepted:  promise.NewEvent(),
		committed: promise.NewEvent(),
		rejected:  promise.NewEvent(),
		evicted:   promise.NewEvent(),

		spent:          promise.NewEvent(),
		doubleSpent:    promise.NewEvent(),
		spendAccepted:  promise.NewEvent1[*TransactionWithMetadata](),
		spendCommitted: promise.NewEvent1[*TransactionWithMetadata](),

		allConsumersEvicted: promise.NewEvent(),
	}
}

func (s *StateWithMetadata) setCommitted() {
	s.committed.Trigger()
}

func (s *StateWithMetadata) setEvicted() {
	s.evicted.Trigger()
}

func (s *StateWithMetadata) OnCommitted(callback func()) {
	s.committed.OnTrigger(callback)
}

func (s *StateWithMetadata) OnEvicted(callback func()) {
	s.evicted.OnTrigger(callback)
}

func (s *StateWithMetadata) OnDoubleSpent(callback func()) {
	s.doubleSpent.OnTrigger(callback)
}

func (s *StateWithMetadata) OnSpendAccepted(callback func(spender *TransactionWithMetadata)) {
	s.spendAccepted.OnTrigger(callback)
}

func (s *StateWithMetadata) OnSpendCommitted(callback func(spender *TransactionWithMetadata)) {
	s.spendCommitted.OnTrigger(callback)
}

func (s *StateWithMetadata) increaseConsumerCount() {
	if atomic.AddUint64(&s.consumerCount, 1) == 2 {
		s.doubleSpent.Trigger()
	}
}

func (s *StateWithMetadata) decreaseConsumerCount() {
	if atomic.AddUint64(&s.consumerCount, ^uint64(0)) == 0 {
		s.allConsumersEvicted.Trigger()
	}
}

func (s *StateWithMetadata) OnAllConsumersEvicted(callback func()) {
	s.allConsumersEvicted.OnTrigger(callback)
}

func (s *StateWithMetadata) AllConsumersEvicted() bool {
	return s.allConsumersEvicted.WasTriggered()
}

func (s *StateWithMetadata) ConsumerCount() uint64 {
	return atomic.LoadUint64(&s.consumerCount)
}

func (s *StateWithMetadata) HasNoConsumers() bool {
	return atomic.LoadUint64(&s.consumerCount) == 0
}

func (s *StateWithMetadata) acceptSpend(spender *TransactionWithMetadata) {
	s.spendAccepted.Trigger(spender)
}

func (s *StateWithMetadata) commitSpend(spender *TransactionWithMetadata) {
	s.spendCommitted.Trigger(spender)
}

func (s *StateWithMetadata) setAccepted() {
	s.accepted.Trigger()
}

func (s *StateWithMetadata) setRejected() {
	s.rejected.Trigger()
}

func (s *StateWithMetadata) OnAccepted(callback func()) {
	s.accepted.OnTrigger(callback)
}

func (s *StateWithMetadata) OnRejected(callback func()) {
	s.rejected.OnTrigger(callback)
}

func (s *StateWithMetadata) IsAccepted() bool {
	return s.accepted.WasTriggered()
}

func (s *StateWithMetadata) ID() iotago.OutputID {
	return s.id
}

func (s *StateWithMetadata) State() ledger.State {
	return s.state
}

func (s *StateWithMetadata) IsSpent() bool {
	return atomic.LoadUint64(&s.consumerCount) > 0
}
