package mempoolv1

import (
	"sync/atomic"

	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/promise"
	"github.com/iotaledger/iota-core/pkg/core/agential"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	iotago "github.com/iotaledger/iota.go/v4"
)

type StateMetadata struct {
	id    iotago.OutputID
	state mempool.State

	// lifecycle
	spenderCount       uint64
	spent              *promise.Event
	doubleSpent        *promise.Event
	spendAccepted      *agential.ValueReceptor[*TransactionMetadata]
	spendCommitted     *agential.ValueReceptor[*TransactionMetadata]
	allSpendersRemoved *event.Event

	conflictIDs *agential.SetReceptor[iotago.TransactionID]

	*inclusionFlags
}

func NewStateMetadata(state mempool.State, optSource ...*TransactionMetadata) *StateMetadata {
	return (&StateMetadata{
		id:    state.OutputID(),
		state: state,

		spent:              promise.NewEvent(),
		doubleSpent:        promise.NewEvent(),
		spendAccepted:      agential.NewValueReceptor[*TransactionMetadata](),
		spendCommitted:     agential.NewValueReceptor[*TransactionMetadata](),
		allSpendersRemoved: event.New(),

		conflictIDs: agential.NewSetReceptor[iotago.TransactionID](),

		inclusionFlags: newInclusionFlags(),
	}).setup(optSource...)
}

func (s *StateMetadata) setup(optSource ...*TransactionMetadata) *StateMetadata {
	if len(optSource) == 0 {
		return s
	}
	source := optSource[0]

	s.conflictIDs.InheritFrom(source.conflictIDs)

	source.OnPending(s.setPending)
	source.OnAccepted(s.setAccepted)
	source.OnRejected(s.setRejected)
	source.OnCommitted(s.setCommitted)
	source.OnOrphaned(s.setOrphaned)

	return s
}

func (s *StateMetadata) ID() iotago.OutputID {
	return s.id
}

func (s *StateMetadata) State() mempool.State {
	return s.state
}

func (s *StateMetadata) ConflictIDs() *agential.SetReceptor[iotago.TransactionID] {
	return s.conflictIDs
}

func (s *StateMetadata) IsDoubleSpent() bool {
	return s.doubleSpent.WasTriggered()
}

func (s *StateMetadata) OnDoubleSpent(callback func()) {
	s.doubleSpent.OnTrigger(callback)
}

func (s *StateMetadata) AcceptedSpender() (mempool.TransactionMetadata, bool) {
	acceptedSpender := s.spendAccepted.Get()

	return acceptedSpender, acceptedSpender != nil
}

func (s *StateMetadata) OnAcceptedSpenderUpdated(callback func(spender mempool.TransactionMetadata)) {
	s.spendAccepted.OnUpdate(func(prevValue, newValue *TransactionMetadata) {
		if prevValue != newValue {
			callback(newValue)
		}
	})
}

func (s *StateMetadata) OnSpendCommitted(callback func(spender mempool.TransactionMetadata)) {
	s.spendCommitted.OnUpdate(func(prevValue, newValue *TransactionMetadata) {
		if prevValue != newValue {
			callback(newValue)
		}
	})
}

func (s *StateMetadata) AllSpendersRemoved() bool {
	return s.allSpendersRemoved.WasTriggered()
}

func (s *StateMetadata) onAllSpendersRemoved(callback func()) (unsubscribe func()) {
	return s.allSpendersRemoved.Hook(callback).Unhook
}

func (s *StateMetadata) PendingSpenderCount() int {
	return int(atomic.LoadUint64(&s.spenderCount))
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
		s.spendAccepted.Set(spender)
	})

	spender.OnPending(func() {
		s.spendAccepted.Set(nil)
	})

	spender.OnCommitted(func() {
		s.spendCommitted.Set(spender)

		s.decreaseSpenderCount()
	})

	spender.OnOrphaned(s.decreaseSpenderCount)
}
