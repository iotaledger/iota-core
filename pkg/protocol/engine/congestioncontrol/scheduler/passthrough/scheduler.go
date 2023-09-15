package passthrough

import (
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/congestioncontrol/scheduler"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Scheduler struct {
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

func New() *Scheduler {
	return &Scheduler{
		events: scheduler.NewEvents(),
	}
}

func (s *Scheduler) Shutdown() {
}

func (s *Scheduler) IsBlockIssuerReady(_ iotago.AccountID, _ ...*blocks.Block) bool {
	return true
}

func (s *Scheduler) BasicBufferSize() int {
	return 0
}

func (s *Scheduler) ValidatorBufferSize() int {
	return 0
}

func (s *Scheduler) ReadyBlocksCount() int {
	return 0
}

func (s *Scheduler) IssuerQueueBlockCount(_ iotago.AccountID) int {
	return 0
}

func (s *Scheduler) ValidatorQueueBlockCount(_ iotago.AccountID) int {
	return 0
}

func (s *Scheduler) IssuerQueueWork(_ iotago.AccountID) iotago.WorkScore {
	return 0
}

func (s *Scheduler) AddBlock(block *blocks.Block) {
	if block.SetScheduled() {
		s.events.BlockScheduled.Trigger(block)
	}
}
