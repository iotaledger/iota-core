package mempoolv1

import (
	"sync/atomic"

	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/promise"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	iotago "github.com/iotaledger/iota.go/v4"
)

type OutputStateMetadata struct {
	outputID iotago.OutputID
	state    mempool.OutputState

	// lifecycle
	spenderCount       uint64
	spent              *promise.Event
	doubleSpent        *promise.Event
	spendAccepted      reactive.Variable[*TransactionMetadata]
	spendCommitted     reactive.Variable[*TransactionMetadata]
	allSpendersRemoved *event.Event

	conflictIDs reactive.DerivedSet[iotago.TransactionID]

	*inclusionFlags
}

func NewOutputStateMetadata(state mempool.OutputState, optSource ...*TransactionMetadata) *OutputStateMetadata {
	return (&OutputStateMetadata{
		outputID: state.OutputID(),
		state:    state,

		spent:              promise.NewEvent(),
		doubleSpent:        promise.NewEvent(),
		spendAccepted:      reactive.NewVariable[*TransactionMetadata](),
		spendCommitted:     reactive.NewVariable[*TransactionMetadata](),
		allSpendersRemoved: event.New(),

		conflictIDs: reactive.NewDerivedSet[iotago.TransactionID](),

		inclusionFlags: newInclusionFlags(),
	}).setup(optSource...)
}

func (s *OutputStateMetadata) setup(optSource ...*TransactionMetadata) *OutputStateMetadata {
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

func (s *OutputStateMetadata) StateID() iotago.Identifier {
	return s.state.StateID()
}

func (s *OutputStateMetadata) Type() iotago.StateType {
	return iotago.InputUTXO
}

func (s *OutputStateMetadata) OutputID() iotago.OutputID {
	return s.outputID
}

func (s *OutputStateMetadata) State() mempool.OutputState {
	return s.state
}

func (s *OutputStateMetadata) ConflictIDs() reactive.Set[iotago.TransactionID] {
	return s.conflictIDs
}

func (s *OutputStateMetadata) IsDoubleSpent() bool {
	return s.doubleSpent.WasTriggered()
}

func (s *OutputStateMetadata) OnDoubleSpent(callback func()) {
	s.doubleSpent.OnTrigger(callback)
}

func (s *OutputStateMetadata) AcceptedSpender() (mempool.TransactionMetadata, bool) {
	acceptedSpender := s.spendAccepted.Get()

	return acceptedSpender, acceptedSpender != nil
}

func (s *OutputStateMetadata) OnAcceptedSpenderUpdated(callback func(spender mempool.TransactionMetadata)) {
	s.spendAccepted.OnUpdate(func(prevValue, newValue *TransactionMetadata) {
		if prevValue != newValue {
			callback(newValue)
		}
	})
}

func (s *OutputStateMetadata) OnSpendCommitted(callback func(spender mempool.TransactionMetadata)) {
	s.spendCommitted.OnUpdate(func(prevValue, newValue *TransactionMetadata) {
		if prevValue != newValue {
			callback(newValue)
		}
	})
}

func (s *OutputStateMetadata) AllSpendersRemoved() bool {
	return s.allSpendersRemoved.WasTriggered()
}

func (s *OutputStateMetadata) onAllSpendersRemoved(callback func()) (unsubscribe func()) {
	return s.allSpendersRemoved.Hook(callback).Unhook
}

func (s *OutputStateMetadata) PendingSpenderCount() int {
	return int(atomic.LoadUint64(&s.spenderCount))
}

func (s *OutputStateMetadata) HasNoSpenders() bool {
	return atomic.LoadUint64(&s.spenderCount) == 0
}

func (s *OutputStateMetadata) increaseSpenderCount() {
	if spenderCount := atomic.AddUint64(&s.spenderCount, 1); spenderCount == 1 {
		s.spent.Trigger()
	} else if spenderCount == 2 {
		s.doubleSpent.Trigger()
	}
}

func (s *OutputStateMetadata) decreaseSpenderCount() {
	if atomic.AddUint64(&s.spenderCount, ^uint64(0)) == 0 {
		s.allSpendersRemoved.Trigger()
	}
}

func (s *OutputStateMetadata) setupSpender(spender *TransactionMetadata) {
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
