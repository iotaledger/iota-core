package mempoolv1

import (
	"sync/atomic"

	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	iotago "github.com/iotaledger/iota.go/v4"
)

type ContextStateMetadata struct {
	state mempool.ContextState

	// lifecycle
	spenderCount       uint64
	allSpendersRemoved *event.Event
}

func NewContextStateMetadata(state mempool.ContextState, optSource ...*TransactionMetadata) *ContextStateMetadata {
	return &ContextStateMetadata{
		state: state,
	}
}

func (s *ContextStateMetadata) State() mempool.ContextState {
	return s.state
}

func (s *ContextStateMetadata) StateID() iotago.Identifier {
	return s.state.StateID()
}

func (s *ContextStateMetadata) Type() iotago.StateType {
	return s.state.Type()
}

func (s *ContextStateMetadata) PendingSpenderCount() int {
	return int(atomic.LoadUint64(&s.spenderCount))
}

func (s *ContextStateMetadata) increaseSpenderCount() {
	atomic.AddUint64(&s.spenderCount, 1)
}

func (s *ContextStateMetadata) decreaseSpenderCount() {
	if atomic.AddUint64(&s.spenderCount, ^uint64(0)) == 0 {
		s.allSpendersRemoved.Trigger()
	}
}

func (s *ContextStateMetadata) onAllSpendersRemoved(callback func()) (unsubscribe func()) {
	return s.allSpendersRemoved.Hook(callback).Unhook
}

func (s *ContextStateMetadata) setupSpender(spender *TransactionMetadata) {
	s.increaseSpenderCount()

	spender.OnCommitted(s.decreaseSpenderCount)

	spender.OnOrphaned(s.decreaseSpenderCount)
}
