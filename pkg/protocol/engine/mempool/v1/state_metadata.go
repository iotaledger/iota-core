package mempoolv1

import (
	"sync/atomic"

	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/iota-core/pkg/core/promise"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledger"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	iotago "github.com/iotaledger/iota.go/v4"
)

type StateMetadata struct {
	id    iotago.OutputID
	state ledger.State

	// lifecycle
	spenderCount       uint64
	spent              *promise.Event
	doubleSpent        *promise.Event
	spendAccepted      *promise.Event1[mempool.TransactionMetadata]
	spendCommitted     *promise.Event1[mempool.TransactionMetadata]
	allSpendersRemoved *event.Event

	*Inclusion
}

func NewStateMetadata(state ledger.State, optSource ...*TransactionMetadata) *StateMetadata {
	return (&StateMetadata{
		id:    state.ID(),
		state: state,

		spent:              promise.NewEvent(),
		doubleSpent:        promise.NewEvent(),
		spendAccepted:      promise.NewEvent1[mempool.TransactionMetadata](),
		spendCommitted:     promise.NewEvent1[mempool.TransactionMetadata](),
		allSpendersRemoved: event.New(),

		Inclusion: NewInclusion(),
	}).setup(optSource...)
}

func (s *StateMetadata) setup(optSource ...*TransactionMetadata) *StateMetadata {
	if len(optSource) > 0 {
		optSource[0].OnPending(s.setPending)
		optSource[0].OnAccepted(s.setAccepted)
		optSource[0].OnRejected(s.setRejected)
		optSource[0].OnCommitted(s.setCommitted)
		optSource[0].OnOrphaned(s.setOrphaned)
	}

	return s
}

func (s *StateMetadata) ID() iotago.OutputID {
	return s.id
}

func (s *StateMetadata) State() ledger.State {
	return s.state
}

func (s *StateMetadata) IsSpent() bool {
	return atomic.LoadUint64(&s.spenderCount) > 0
}

func (s *StateMetadata) OnDoubleSpent(callback func()) {
	s.doubleSpent.OnTrigger(callback)
}

func (s *StateMetadata) OnSpendAccepted(callback func(spender mempool.TransactionMetadata)) {
	s.spendAccepted.OnTrigger(callback)
}

func (s *StateMetadata) OnSpendCommitted(callback func(spender mempool.TransactionMetadata)) {
	s.spendCommitted.OnTrigger(callback)
}

func (s *StateMetadata) AllSpendersRemoved() bool {
	return s.allSpendersRemoved.WasTriggered()
}

func (s *StateMetadata) OnAllSpendersRemoved(callback func()) (unsubscribe func()) {
	return s.allSpendersRemoved.Hook(callback).Unhook
}

func (s *StateMetadata) SpenderCount() uint64 {
	return atomic.LoadUint64(&s.spenderCount)
}

func (s *StateMetadata) HasNoSpenders() bool {
	return atomic.LoadUint64(&s.spenderCount) == 0
}

func (s *StateMetadata) increaseSpenderCount() {
	if spenderCount := atomic.AddUint64(&s.spenderCount, 1); spenderCount == 1 {
		s.spent.Trigger()
	} else if spenderCount == 2 {
		s.doubleSpent.Trigger()
	}
}

func (s *StateMetadata) decreaseSpenderCount() {
	if atomic.AddUint64(&s.spenderCount, ^uint64(0)) == 0 {
		s.allSpendersRemoved.Trigger()
	}
}

func (s *StateMetadata) setupSpender(spender *TransactionMetadata) {
	s.increaseSpenderCount()

	spender.OnAccepted(func() {
		s.spendAccepted.Trigger(spender)
	})

	spender.OnCommitted(func() {
		s.spendCommitted.Trigger(spender)

		s.decreaseSpenderCount()
	})

	spender.OnOrphaned(s.decreaseSpenderCount)
}
