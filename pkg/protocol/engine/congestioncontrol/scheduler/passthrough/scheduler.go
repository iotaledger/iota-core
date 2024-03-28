package passthrough

import (
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
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
		s := New(e.NewSubModule("Scheduler"))

		e.ConstructedEvent().OnTrigger(func() {
			e.Events.Booker.BlockBooked.Hook(func(block *blocks.Block) {
				s.AddBlock(block)
			})

			e.Events.Scheduler.LinkTo(s.events)

			s.InitializedEvent().Trigger()
		})

		return s
	})
}

func New(subModule module.Module) *Scheduler {
	return options.Apply(&Scheduler{
		Module: subModule,
		events: scheduler.NewEvents(),
	}, nil, func(s *Scheduler) {
		s.ShutdownEvent().OnTrigger(func() {
			s.StoppedEvent().Trigger()
		})

		s.ConstructedEvent().Trigger()
	})
}

func (s *Scheduler) IsBlockIssuerReady(_ iotago.AccountID, _ ...iotago.WorkScore) bool {
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

// Reset resets the component to a clean state as if it was created at the last commitment.
func (s *Scheduler) Reset() { /* nothing to reset but comply with interface */ }
