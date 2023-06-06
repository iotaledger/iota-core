package blocktime

import (
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/clock"
	iotago "github.com/iotaledger/iota.go/v4"
)

// Clock implements the clock.Clock interface that sources its notion of time from accepted and confirmed blocks.
type Clock struct {
	// preAcceptedTime contains a notion of time that is anchored to the latest preAccepted block.
	preAcceptedTime *RelativeTime

	// acceptedTime contains a notion of time that is anchored to the latest accepted block.
	acceptedTime *RelativeTime

	// preConfirmedTime contains a notion of time that is anchored to the latest preConfirmed block.
	preConfirmedTime *RelativeTime

	// confirmedTime contains a notion of time that is anchored to the latest confirmed block.
	confirmedTime *RelativeTime

	// Module embeds the required methods of the module.Interface.
	module.Module
}

// NewProvider creates a new Clock provider with the given options.
func NewProvider(opts ...options.Option[Clock]) module.Provider[*engine.Engine, clock.Clock] {
	return module.Provide(func(e *engine.Engine) clock.Clock {
		return options.Apply(&Clock{
			preAcceptedTime:  NewRelativeTime(),
			acceptedTime:     NewRelativeTime(),
			preConfirmedTime: NewRelativeTime(),
			confirmedTime:    NewRelativeTime(),
		}, opts, func(c *Clock) {
			e.HookConstructed(func() {
				e.Storage.Settings().HookInitialized(func() {
					c.preAcceptedTime.Set(e.API().SlotTimeProvider().EndTime(e.Storage.Settings().LatestCommitment().Index()))
					c.acceptedTime.Set(e.API().SlotTimeProvider().EndTime(e.Storage.Settings().LatestCommitment().Index()))

					// TODO: should this be last finalized slot?
					c.preConfirmedTime.Set(e.API().SlotTimeProvider().EndTime(e.Storage.Settings().LatestCommitment().Index()))
					c.confirmedTime.Set(e.API().SlotTimeProvider().EndTime(e.Storage.Settings().LatestCommitment().Index()))

					c.TriggerInitialized()
				})

				e.Events.Clock.PreAcceptedTimeUpdated.LinkTo(c.preAcceptedTime.OnUpdated)
				e.Events.Clock.AcceptedTimeUpdated.LinkTo(c.acceptedTime.OnUpdated)
				e.Events.Clock.PreConfirmedTimeUpdated.LinkTo(c.preConfirmedTime.OnUpdated)
				e.Events.Clock.ConfirmedTimeUpdated.LinkTo(c.confirmedTime.OnUpdated)

				asyncOpt := event.WithWorkerPool(e.Workers.CreatePool("Clock", 1))
				c.HookStopped(lo.Batch(
					e.Events.BlockGadget.BlockPreAccepted.Hook(func(block *blocks.Block) {
						c.preAcceptedTime.Advance(block.IssuingTime())
					}, asyncOpt).Unhook,

					e.Events.BlockGadget.BlockAccepted.Hook(func(block *blocks.Block) {
						c.preAcceptedTime.Advance(block.IssuingTime())
						c.acceptedTime.Advance(block.IssuingTime())
					}, asyncOpt).Unhook,

					e.Events.BlockGadget.BlockPreConfirmed.Hook(func(block *blocks.Block) {
						c.preAcceptedTime.Advance(block.IssuingTime())
						c.preConfirmedTime.Advance(block.IssuingTime())
					}, asyncOpt).Unhook,

					e.Events.BlockGadget.BlockConfirmed.Hook(func(block *blocks.Block) {
						c.preAcceptedTime.Advance(block.IssuingTime())
						c.acceptedTime.Advance(block.IssuingTime())
						c.preConfirmedTime.Advance(block.IssuingTime())
						c.confirmedTime.Advance(block.IssuingTime())
					}, asyncOpt).Unhook,

					e.Events.SlotGadget.SlotFinalized.Hook(func(index iotago.SlotIndex) {
						c.preAcceptedTime.Advance(e.API().SlotTimeProvider().EndTime(index))
						c.acceptedTime.Advance(e.API().SlotTimeProvider().EndTime(index))
						c.preConfirmedTime.Advance(e.API().SlotTimeProvider().EndTime(index))
						c.confirmedTime.Advance(e.API().SlotTimeProvider().EndTime(index))
					}, asyncOpt).Unhook,
				))
			})

			e.HookStopped(c.TriggerStopped)
		}, (*Clock).TriggerConstructed)
	})
}

// PreAccepted returns a notion of time that is anchored to the latest preAccepted block.
func (c *Clock) PreAccepted() clock.RelativeTime {
	return c.preAcceptedTime
}

// Accepted returns a notion of time that is anchored to the latest accepted block.
func (c *Clock) Accepted() clock.RelativeTime {
	return c.acceptedTime
}

// PreConfirmed returns a notion of time that is anchored to the latest preConfirmed block.
func (c *Clock) PreConfirmed() clock.RelativeTime {
	return c.preConfirmedTime
}

// Confirmed returns a notion of time that is anchored to the latest confirmed block.
func (c *Clock) Confirmed() clock.RelativeTime {
	return c.confirmedTime
}

func (c *Clock) Shutdown() {
	c.TriggerStopped()
}
