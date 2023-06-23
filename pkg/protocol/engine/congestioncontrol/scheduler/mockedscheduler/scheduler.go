package mockedscheduler

import (
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/congestioncontrol/scheduler"
	iotago "github.com/iotaledger/iota.go/v4"
)

type MockedScheduler struct {
	events *scheduler.Events

	module.Module
}

func NewProvider() module.Provider[*engine.Engine, scheduler.Scheduler] {
	return module.Provide(func(e *engine.Engine) scheduler.Scheduler {
		s := New()
		e.HookConstructed(func() {
			e.Events.Scheduler.LinkTo(s.events)
			s.TriggerConstructed()
			e.Events.Booker.BlockBooked.Hook(func(block *blocks.Block) {
				s.AddBlock(block)
			})
		})

		return s
	})
}

func New() *MockedScheduler {
	return &MockedScheduler{
		events: scheduler.NewEvents(),
	}
}

func (s *MockedScheduler) Shutdown() {
}

func (s *MockedScheduler) IsBlockIssuerReady(_ iotago.AccountID, _ ...*blocks.Block) bool {
	return true
}

func (s *MockedScheduler) AddBlock(block *blocks.Block) {
	if block.SetScheduled() {
		s.events.BlockScheduled.Trigger(block)
	}
}
