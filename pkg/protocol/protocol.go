package protocol

import (
	"context"
	"fmt"

	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/network"
)

type Protocol struct {
	workers *workerpool.Group
	status  reactive.Variable[Status]
	error   *event.Event1[error]
	options *Options

	*Network
	*Engines
	*Chains
	*Dispatcher
	*APIProvider

	module.Module
}

func New(workers *workerpool.Group, dispatcher network.Endpoint, opts ...options.Option[Protocol]) *Protocol {
	return options.Apply(&Protocol{
		workers: workers,
		error:   event.New1[error](),
		options: newOptions(),
	}, opts, func(p *Protocol) {
		p.status = newStatusVariable(p)
		p.Network = newNetwork(p, dispatcher)
		p.Engines = newEngines(p)
		p.Chains = newChains(p)
		p.Dispatcher = newDispatcher(p)
		p.APIProvider = newAPIProvider(p)
	}, (*Protocol).TriggerConstructed)
}

func (p *Protocol) Run(ctx context.Context) error {
	defer p.TriggerStopped()

	p.TriggerInitialized()

	fmt.Println("RUN STARTED")

	<-ctx.Done()

	fmt.Println("RUN FINISHED")

	p.TriggerShutdown()

	return nil
}

func (p *Protocol) Workers() *workerpool.Group {
	return p.workers
}

func (p *Protocol) Status() Status {
	return p.status.Get()
}

func (p *Protocol) StatusR() reactive.Variable[Status] {
	return p.status
}

func (p *Protocol) OnError(callback func(error)) (unsubscribe func()) {
	return p.error.Hook(callback).Unhook
}

func (p *Protocol) TriggerError(err error) {
	p.error.Trigger(err)
}
