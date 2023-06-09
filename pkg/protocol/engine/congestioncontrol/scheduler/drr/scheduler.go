package drr

import (
	"math/big"
	"sync"
	"time"

	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/congestioncontrol/scheduler"

	iotago "github.com/iotaledger/iota.go/v4"
)

type Scheduler struct {
	events *scheduler.Events

	optsRate                           time.Duration
	optsMaxBufferSize                  int
	optsAcceptedBlockScheduleThreshold time.Duration
	optsMaxDeficit                     *big.Rat

	bufferMutex sync.RWMutex

	module.Module
}

func NewProvider(opts ...options.Option[Scheduler]) module.Provider[*engine.Engine, scheduler.Scheduler] {
	return module.Provide(func(e *engine.Engine) scheduler.Scheduler {
		s := New(opts...)

		e.Events.Booker.BlockBooked.Hook(s.AddBlock, event.WithWorkerPool(e.Workers.CreatePool("Enqueue")))
		e.Events.Scheduler.LinkTo(s.events)

		s.HookInitialized(s.Start, event.WithWorkerPool(e.Workers.CreatePool("Scheduler")))
		s.TriggerInitialized()

		return s
	})
}

func New(opts ...options.Option[Scheduler]) *Scheduler {
	return options.Apply(&Scheduler{}, opts, (*Scheduler).TriggerConstructed)
}

func (s *Scheduler) Shutdown() {
	s.TriggerStopped()
}

// Start starts the scheduler.
func (s *Scheduler) Start() {
	go s.mainLoop()
}

// Rate gets the rate of the scheduler.
func (s *Scheduler) Rate() time.Duration {
	return s.optsRate
}

func (s *Scheduler) IsBlockIssuerReady(accountID iotago.AccountID) bool {
	if s.IsUncongested() {
		return true
	} else {
		// Note: this method needs to be updated to take the expected work of the incoming block as an argument.
		expectedWork := iotago.MaxBlockSize
		excessDeficit, _ := s.getExcessDeficit(accountID)
		return excessDeficit >= float64(expectedWork)
	}
}

// IsUncongested checks if the issuerQueue is completely uncongested.
func (s *Scheduler) IsUncongested() bool {
	return s.ReadyBlocksCount() == 0
}

func (s *Scheduler) AddBlock(*blocks.Block) {
	s.bufferMutex.Lock()
	defer s.bufferMutex.Unlock()

}

// region Options ////////////////////////////////////////////////////////////////////////////////////////////////////

func WithAcceptedBlockScheduleThreshold(acceptedBlockScheduleThreshold time.Duration) options.Option[Scheduler] {
	return func(s *Scheduler) {
		s.optsAcceptedBlockScheduleThreshold = acceptedBlockScheduleThreshold
	}
}

func WithMaxBufferSize(maxBufferSize int) options.Option[Scheduler] {
	return func(s *Scheduler) {
		s.optsMaxBufferSize = maxBufferSize
	}
}

func WithRate(rate time.Duration) options.Option[Scheduler] {
	return func(s *Scheduler) {
		s.optsRate = rate
	}
}

func WithMaxDeficit(maxDef int) options.Option[Scheduler] {
	return func(s *Scheduler) {
		s.optsMaxDeficit = new(big.Rat).SetInt64(int64(maxDef))
	}
}
