package blocktime

import (
	"time"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/syncutils"

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

	syncutils.RWMutex

	// Module embeds the required methods of the module.Interface.
	module.Module
}

// NewProvider creates a new Clock provider with the given options.
func NewProvider(opts ...options.Option[Clock]) module.Provider[*engine.Engine, clock.Clock] {
	return module.Provide(func(e *engine.Engine) clock.Clock {
		return options.Apply(&Clock{
			acceptedTime:  NewRelativeTime(),
			confirmedTime: NewRelativeTime(),
		}, opts, func(c *Clock) {
			e.HookConstructed(func() {
				latestCommitmentIndex := e.Storage.Settings().LatestCommitment().Index()
				c.acceptedTime.Set(e.APIForSlot(latestCommitmentIndex).TimeProvider().SlotEndTime(latestCommitmentIndex))

				latestFinalizedSlotIndex := e.Storage.Settings().LatestFinalizedSlot()
				c.confirmedTime.Set(e.APIForSlot(latestFinalizedSlotIndex).TimeProvider().SlotEndTime(latestFinalizedSlotIndex))

				c.TriggerInitialized()

				e.Events.Clock.AcceptedTimeUpdated.LinkTo(c.acceptedTime.OnUpdated)
				e.Events.Clock.ConfirmedTimeUpdated.LinkTo(c.confirmedTime.OnUpdated)

				asyncOpt := event.WithWorkerPool(e.Workers.CreatePool("Clock", 1))
				c.HookStopped(lo.Batch(
					e.Events.BlockGadget.BlockAccepted.Hook(func(block *blocks.Block) {
						c.advanceAccepted(block.IssuingTime())
					}, asyncOpt).Unhook,

					e.Events.BlockGadget.BlockConfirmed.Hook(func(block *blocks.Block) {
						c.advanceConfirmed(block.IssuingTime())
					}, asyncOpt).Unhook,

					e.Events.SlotGadget.SlotFinalized.Hook(func(slot iotago.SlotIndex) {
						timeProvider := e.APIForSlot(slot).TimeProvider()
						slotEndTime := timeProvider.SlotEndTime(slot)

						c.onSlotFinalized(slotEndTime)
					}, asyncOpt).Unhook,
				))
			})

			e.HookStopped(c.TriggerStopped)
		}, (*Clock).TriggerConstructed)
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

func (c *Clock) Shutdown() {
	c.TriggerStopped()
}

func (c *Clock) advanceAccepted(time time.Time) {
	c.Lock()
	defer c.Unlock()

	c.acceptedTime.Advance(time)
}

func (c *Clock) advanceConfirmed(time time.Time) {
	c.Lock()
	defer c.Unlock()

	c.confirmedTime.Advance(time)
}

func (c *Clock) onSlotFinalized(slotEndTime time.Time) {
	c.Lock()
	defer c.Unlock()

	c.acceptedTime.Advance(slotEndTime)
	c.confirmedTime.Advance(slotEndTime)
}
