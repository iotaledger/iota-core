package mempoolv1

import (
	"sync/atomic"

	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/iota-core/pkg/core/promise"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
)

type StateLifecycle struct {
	spenderCount       uint64
	spent              *promise.Event
	doubleSpent        *promise.Event
	spendAccepted      *promise.Event1[mempool.TransactionWithMetadata]
	spendCommitted     *promise.Event1[mempool.TransactionWithMetadata]
	allSpendersRemoved *event.Event
}

func NewStateLifecycle() *StateLifecycle {
	return &StateLifecycle{
		spent:              promise.NewEvent(),
		doubleSpent:        promise.NewEvent(),
		spendAccepted:      promise.NewEvent1[mempool.TransactionWithMetadata](),
		spendCommitted:     promise.NewEvent1[mempool.TransactionWithMetadata](),
		allSpendersRemoved: event.New(),
	}
}

func (s *StateLifecycle) IsSpent() bool {
	return atomic.LoadUint64(&s.spenderCount) > 0
}

func (s *StateLifecycle) OnDoubleSpent(callback func()) {
	s.doubleSpent.OnTrigger(callback)
}

func (s *StateLifecycle) OnSpendAccepted(callback func(spender mempool.TransactionWithMetadata)) {
	s.spendAccepted.OnTrigger(callback)
}

func (s *StateLifecycle) OnSpendCommitted(callback func(spender mempool.TransactionWithMetadata)) {
	s.spendCommitted.OnTrigger(callback)
}

func (s *StateLifecycle) AllSpendersRemoved() bool {
	return s.allSpendersRemoved.WasTriggered()
}

func (s *StateLifecycle) OnAllSpendersRemoved(callback func()) (unsubscribe func()) {
	return s.allSpendersRemoved.Hook(callback).Unhook
}

func (s *StateLifecycle) SpenderCount() uint64 {
	return atomic.LoadUint64(&s.spenderCount)
}

func (s *StateLifecycle) HasNoSpenders() bool {
	return atomic.LoadUint64(&s.spenderCount) == 0
}

func (s *StateLifecycle) increaseSpenderCount() {
	if spenderCount := atomic.AddUint64(&s.spenderCount, 1); spenderCount == 1 {
		s.spent.Trigger()
	} else if spenderCount == 2 {
		s.doubleSpent.Trigger()
	}
}

func (s *StateLifecycle) decreaseSpenderCount() {
	if atomic.AddUint64(&s.spenderCount, ^uint64(0)) == 0 {
		s.allSpendersRemoved.Trigger()
	}
}

func (s *StateLifecycle) dependsOnSpender(spender *TransactionMetadata) {
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
