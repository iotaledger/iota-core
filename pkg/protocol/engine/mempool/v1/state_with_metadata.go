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
	spenderCount      uint64
	state             ledger.State

	accepted      *promise.Event
	rejected      *promise.Event
	doubleSpent   *promise.Event
	spendAccepted *promise.Event1[*TransactionWithMetadata]
}

func NewStateWithMetadata(state ledger.State, optSource ...*TransactionWithMetadata) *StateWithMetadata {
	return &StateWithMetadata{
		id:                state.ID(),
		sourceTransaction: lo.First(optSource),
		state:             state,
		accepted:          promise.NewEvent(),
		rejected:          promise.NewEvent(),
		doubleSpent:       promise.NewEvent(),
		spendAccepted:     promise.NewEvent1[*TransactionWithMetadata](),
	}
}

func (s *StateWithMetadata) OnDoubleSpent(callback func()) {
	s.doubleSpent.OnTrigger(callback)
}

func (s *StateWithMetadata) OnSpendAccepted(callback func(spender *TransactionWithMetadata)) {
	s.spendAccepted.OnTrigger(callback)
}

func (s *StateWithMetadata) markSpent() {
	if atomic.AddUint64(&s.spenderCount, 1) == 2 {
		s.doubleSpent.Trigger()
	}
}

func (s *StateWithMetadata) acceptSpend(spender *TransactionWithMetadata) {
	s.spendAccepted.Trigger(spender)
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
	return atomic.LoadUint64(&s.spenderCount) > 0
}
