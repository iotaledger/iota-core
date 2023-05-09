package mempoolv1

import (
	"sync/atomic"

	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/iota-core/pkg/core/promise"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
)

type SpentState struct {
	spenderCount       uint64
	spent              *promise.Event
	doubleSpent        *promise.Event
	spendAccepted      *promise.Event1[mempool.TransactionWithMetadata]
	spendCommitted     *promise.Event1[mempool.TransactionWithMetadata]
	allSpendersRemoved *event.Event
}

func NewSpentState() *SpentState {
	return &SpentState{
		spent:              promise.NewEvent(),
		doubleSpent:        promise.NewEvent(),
		spendAccepted:      promise.NewEvent1[mempool.TransactionWithMetadata](),
		spendCommitted:     promise.NewEvent1[mempool.TransactionWithMetadata](),
		allSpendersRemoved: event.New(),
	}
}

func (s *SpentState) IsSpent() bool {
	return atomic.LoadUint64(&s.spenderCount) > 0
}

func (s *SpentState) OnDoubleSpent(callback func()) {
	s.doubleSpent.OnTrigger(callback)
}

func (s *SpentState) OnSpendAccepted(callback func(spender mempool.TransactionWithMetadata)) {
	s.spendAccepted.OnTrigger(callback)
}

func (s *SpentState) OnSpendCommitted(callback func(spender mempool.TransactionWithMetadata)) {
	s.spendCommitted.OnTrigger(callback)
}

func (s *SpentState) AllSpendersRemoved() bool {
	return s.allSpendersRemoved.WasTriggered()
}

func (s *SpentState) OnAllSpendersRemoved(callback func()) (unsubscribe func()) {
	return s.allSpendersRemoved.Hook(callback).Unhook
}

func (s *SpentState) SpenderCount() uint64 {
	return atomic.LoadUint64(&s.spenderCount)
}

func (s *SpentState) HasNoSpenders() bool {
	return atomic.LoadUint64(&s.spenderCount) == 0
}

func (s *SpentState) increaseSpenderCount() {
	if spenders := atomic.AddUint64(&s.spenderCount, 1); spenders == 1 {
		s.spent.Trigger()
	} else if spenders == 2 {
		s.doubleSpent.Trigger()
	}
}

func (s *SpentState) decreaseSpenderCount() {
	if atomic.AddUint64(&s.spenderCount, ^uint64(0)) == 0 {
		s.allSpendersRemoved.Trigger()
	}
}

func (s *SpentState) acceptSpend(spender *TransactionWithMetadata) {
	s.spendAccepted.Trigger(spender)
}

func (s *SpentState) commitSpend(spender *TransactionWithMetadata) {
	s.spendCommitted.Trigger(spender)
}
