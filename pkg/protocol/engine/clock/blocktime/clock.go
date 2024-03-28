package blocktime

import (
	"time"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/hive.go/runtime/workerpool"

	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/clock"
	iotago "github.com/iotaledger/iota.go/v4"
)

// Clock implements the clock.Clock interface that sources its notion of time from accepted and confirmed blocks.
type Clock struct {
	// acceptedTime contains a notion of time that is anchored to the latest accepted block.
	acceptedTime *RelativeTime

	// confirmedTime contains a notion of time that is anchored to the latest confirmed block.
	confirmedTime *RelativeTime

	workerPool *workerpool.WorkerPool

	syncutils.RWMutex

	// Module embeds the required methods of the modular framework.
	module.Module
}

// NewProvider creates a new Clock provider with the given options.
func NewProvider(opts ...options.Option[Clock]) module.Provider[*engine.Engine, clock.Clock] {
	return module.Provide(func(e *engine.Engine) clock.Clock {
		c := New(e.NewSubModule("Clock"), e, opts...)

		e.ConstructedEvent().OnTrigger(func() {
			latestCommitmentIndex := e.Storage.Settings().LatestCommitment().Slot()
			c.acceptedTime.Set(e.APIForSlot(latestCommitmentIndex).TimeProvider().SlotEndTime(latestCommitmentIndex))

			latestFinalizedSlotIndex := e.Storage.Settings().LatestFinalizedSlot()
			c.confirmedTime.Set(e.APIForSlot(latestFinalizedSlotIndex).TimeProvider().SlotEndTime(latestFinalizedSlotIndex))

			e.Events.Clock.AcceptedTimeUpdated.LinkTo(c.acceptedTime.OnUpdated)
			e.Events.Clock.ConfirmedTimeUpdated.LinkTo(c.confirmedTime.OnUpdated)

			asyncOpt := event.WithWorkerPool(c.workerPool)

			unhook := lo.Batch(
				e.Events.BlockGadget.BlockAccepted.Hook(func(block *blocks.Block) {
					c.acceptedTime.Advance(block.IssuingTime())
				}, asyncOpt).Unhook,

				e.Events.BlockGadget.BlockConfirmed.Hook(func(block *blocks.Block) {
					c.confirmedTime.Advance(block.IssuingTime())
				}, asyncOpt).Unhook,

				e.Events.SlotGadget.SlotFinalized.Hook(func(slot iotago.SlotIndex) {
					timeProvider := e.APIForSlot(slot).TimeProvider()
					slotEndTime := timeProvider.SlotEndTime(slot)

					c.acceptedTime.Advance(slotEndTime)
					c.confirmedTime.Advance(slotEndTime)
				}, asyncOpt).Unhook,
			)

			c.ShutdownEvent().OnTrigger(func() {
				unhook()
				c.workerPool.Shutdown()

				c.StoppedEvent().Trigger()
			})

			c.InitializedEvent().Trigger()
		})

		return c
	})
}

func New(subModule module.Module, engine *engine.Engine, opts ...options.Option[Clock]) *Clock {
	return options.Apply(&Clock{
		Module:        subModule,
		acceptedTime:  NewRelativeTime(),
		confirmedTime: NewRelativeTime(),
		workerPool:    engine.Workers.CreatePool("Clock", workerpool.WithWorkerCount(1), workerpool.WithCancelPendingTasksOnShutdown(true), workerpool.WithPanicOnSubmitAfterShutdown(true)),
	}, opts, func(c *Clock) {
		c.ShutdownEvent().OnTrigger(func() {
			c.workerPool.Shutdown()
		})

		c.ConstructedEvent().Trigger()
	})
}

// Accepted returns a notion of time that is anchored to the latest accepted block.
func (c *Clock) Accepted() clock.RelativeTime {
	return c.acceptedTime
}

// Confirmed returns a notion of time that is anchored to the latest confirmed block.
func (c *Clock) Confirmed() clock.RelativeTime {
	return c.confirmedTime
}

func (c *Clock) Snapshot() *clock.Snapshot {
	c.RLock()
	defer c.RUnlock()

	return &clock.Snapshot{
		AcceptedTime:          c.acceptedTime.Time(),
		RelativeAcceptedTime:  c.acceptedTime.RelativeTime(),
		ConfirmedTime:         c.confirmedTime.Time(),
		RelativeConfirmedTime: c.confirmedTime.RelativeTime(),
	}
}

// Reset resets the time values tracked in the clock to the given time.
func (c *Clock) Reset(newTime time.Time) {
	c.acceptedTime.Reset(newTime)
	c.confirmedTime.Reset(newTime)
}
