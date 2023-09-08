package protocol

import (
	"context"

	"github.com/iotaledger/hive.go/logger"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/network"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Protocol struct {
	Events *Events

	Workers *workerpool.Group
	error   *event.Event1[error]
	options *Options

	*Network
	*Engines
	*Chains
	*Gossip

	*logger.WrappedLogger
	module.Module
}

func New(loggerInstance *logger.Logger, workers *workerpool.Group, dispatcher network.Endpoint, opts ...options.Option[Protocol]) *Protocol {
	return options.Apply(&Protocol{
		Events:        NewEvents(),
		Workers:       workers,
		WrappedLogger: logger.NewWrappedLogger(loggerInstance),
		error:         event.New1[error](),
		options:       newOptions(),
	}, opts, func(p *Protocol) {
		p.Network = newNetwork(p, dispatcher)
		p.Engines = newEngines(p)
		p.Chains = newChains(p)
		p.Gossip = NewGossip(p)
	}, func(p *Protocol) {
		p.HookShutdown(workers.Shutdown)
	}, (*Protocol).TriggerConstructed)
}

func (p *Protocol) Run(ctx context.Context) error {
	p.TriggerInitialized()

	<-ctx.Done()

	p.TriggerShutdown()
	p.TriggerStopped()

	return ctx.Err()
}

// APIForVersion returns the API for the given version.
func (p *Protocol) APIForVersion(version iotago.Version) (api iotago.API, err error) {
	return p.MainEngineInstance().APIForVersion(version)
}

// APIForSlot returns the API for the given slot.
func (p *Protocol) APIForSlot(slot iotago.SlotIndex) iotago.API {
	return p.MainEngineInstance().APIForSlot(slot)
}

func (p *Protocol) APIForEpoch(epoch iotago.EpochIndex) iotago.API {
	return p.MainEngineInstance().APIForEpoch(epoch)
}

func (p *Protocol) CurrentAPI() iotago.API {
	return p.MainEngineInstance().CurrentAPI()
}

func (p *Protocol) LatestAPI() iotago.API {
	return p.MainEngineInstance().LatestAPI()
}

func (p *Protocol) OnError(callback func(error)) (unsubscribe func()) {
	return p.error.Hook(callback).Unhook
}
