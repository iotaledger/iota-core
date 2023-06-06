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
	// acceptedTime contains a notion of time that is anchored to the latest accepted block.
	acceptedTime *RelativeTime

	// ratifiedAcceptedTime contains a notion of time that is anchored to the latest ratified accepted block.
	ratifiedAcceptedTime *RelativeTime

	// confirmedTime contains a notion of time that is anchored to the latest confirmed block.
	confirmedTime *RelativeTime

	// ratifiedConfirmedTime contains a notion of time that is anchored to the latest ratified confirmed block.
	ratifiedConfirmedTime *RelativeTime

	// Module embeds the required methods of the module.Interface.
	module.Module
}

// NewProvider creates a new Clock provider with the given options.
func NewProvider(opts ...options.Option[Clock]) module.Provider[*engine.Engine, clock.Clock] {
	return module.Provide(func(e *engine.Engine) clock.Clock {
		return options.Apply(&Clock{
			acceptedTime:          NewRelativeTime(),
			ratifiedAcceptedTime:  NewRelativeTime(),
			confirmedTime:         NewRelativeTime(),
			ratifiedConfirmedTime: NewRelativeTime(),
		}, opts, func(c *Clock) {
			e.HookConstructed(func() {
				e.Storage.Settings().HookInitialized(func() {
					c.acceptedTime.Set(e.API().SlotTimeProvider().EndTime(e.Storage.Settings().LatestCommitment().Index()))
					c.ratifiedAcceptedTime.Set(e.API().SlotTimeProvider().EndTime(e.Storage.Settings().LatestCommitment().Index()))

					// TODO: should this be last finalized slot?
					c.confirmedTime.Set(e.API().SlotTimeProvider().EndTime(e.Storage.Settings().LatestCommitment().Index()))
					c.ratifiedConfirmedTime.Set(e.API().SlotTimeProvider().EndTime(e.Storage.Settings().LatestCommitment().Index()))

					c.TriggerInitialized()
				})

				e.Events.Clock.AcceptedTimeUpdated.LinkTo(c.acceptedTime.OnUpdated)
				e.Events.Clock.RatifiedAcceptedTimeUpdated.LinkTo(c.ratifiedAcceptedTime.OnUpdated)
				e.Events.Clock.ConfirmedTimeUpdated.LinkTo(c.confirmedTime.OnUpdated)
				e.Events.Clock.RatifiedConfirmedTimeUpdated.LinkTo(c.ratifiedConfirmedTime.OnUpdated)

				asyncOpt := event.WithWorkerPool(e.Workers.CreatePool("Clock", 1))
				c.HookStopped(lo.Batch(
					e.Events.BlockGadget.BlockAccepted.Hook(func(block *blocks.Block) {
						c.acceptedTime.Advance(block.IssuingTime())
					}, asyncOpt).Unhook,

					e.Events.BlockGadget.BlockRatifiedAccepted.Hook(func(block *blocks.Block) {
						c.acceptedTime.Advance(block.IssuingTime())
						c.ratifiedAcceptedTime.Advance(block.IssuingTime())
					}, asyncOpt).Unhook,

					e.Events.BlockGadget.BlockConfirmed.Hook(func(block *blocks.Block) {
						c.acceptedTime.Advance(block.IssuingTime())
						c.confirmedTime.Advance(block.IssuingTime())
					}, asyncOpt).Unhook,

					e.Events.BlockGadget.BlockRatifiedConfirmed.Hook(func(block *blocks.Block) {
						c.acceptedTime.Advance(block.IssuingTime())
						c.ratifiedAcceptedTime.Advance(block.IssuingTime())
						c.confirmedTime.Advance(block.IssuingTime())
						c.ratifiedConfirmedTime.Advance(block.IssuingTime())
					}, asyncOpt).Unhook,

					e.Events.SlotGadget.SlotFinalized.Hook(func(index iotago.SlotIndex) {
						c.acceptedTime.Advance(e.API().SlotTimeProvider().EndTime(index))
						c.ratifiedAcceptedTime.Advance(e.API().SlotTimeProvider().EndTime(index))
						c.confirmedTime.Advance(e.API().SlotTimeProvider().EndTime(index))
						c.ratifiedConfirmedTime.Advance(e.API().SlotTimeProvider().EndTime(index))
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

// RatifiedAccepted returns a notion of time that is anchored to the latest ratified accepted block.
func (c *Clock) RatifiedAccepted() clock.RelativeTime {
	return c.ratifiedAcceptedTime
}

// Confirmed returns a notion of time that is anchored to the latest confirmed block.
func (c *Clock) Confirmed() clock.RelativeTime {
	return c.confirmedTime
}

// RatifiedConfirmed returns a notion of time that is anchored to the latest confirmed block.
func (c *Clock) RatifiedConfirmed() clock.RelativeTime {
	return c.ratifiedConfirmedTime
}

func (c *Clock) Shutdown() {
	c.TriggerStopped()
}
