package protocol

import (
	"context"

	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/network"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Protocol struct {
	workers *workerpool.Group
	error   *event.Event1[error]
	options *Options

	*Network
	*Engines
	*Chains
	*Gossip

	module.Module
}

func New(workers *workerpool.Group, dispatcher network.Endpoint, opts ...options.Option[Protocol]) *Protocol {
	return options.Apply(&Protocol{
		workers: workers,
		error:   event.New1[error](),
		options: newOptions(),
	}, opts, func(p *Protocol) {
		p.Network = newNetwork(p, dispatcher)
		p.Engines = newEngines(p)
		p.Chains = newChains(p)
		p.Gossip = NewGossip(p)
	}, (*Protocol).TriggerConstructed)
}

func (p *Protocol) Run(ctx context.Context) error {
	defer p.TriggerStopped()

	p.TriggerInitialized()

	<-ctx.Done()

	p.TriggerShutdown()

	return ctx.Err()
}

func (p *Protocol) Workers() *workerpool.Group {
	return p.workers
}

// APIForVersion returns the API for the given version.
func (p *Protocol) APIForVersion(version iotago.Version) (api iotago.API, err error) {
	return p.MainEngine().APIForVersion(version)
}

// APIForSlot returns the API for the given slot.
func (p *Protocol) APIForSlot(slot iotago.SlotIndex) iotago.API {
	return p.MainEngine().APIForSlot(slot)
}

func (p *Protocol) APIForEpoch(epoch iotago.EpochIndex) iotago.API {
	return p.MainEngine().APIForEpoch(epoch)
}

func (p *Protocol) CurrentAPI() iotago.API {
	return p.CurrentAPI()
}

func (p *Protocol) LatestAPI() iotago.API {
	return p.MainEngine().LatestAPI()
}

func (p *Protocol) OnError(callback func(error)) (unsubscribe func()) {
	return p.error.Hook(callback).Unhook
}

func (p *Protocol) TriggerError(err error) {
	p.error.Trigger(err)
}
