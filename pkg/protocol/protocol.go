package protocol

import (
	"context"
	"sync"

	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/network"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
)

type Protocol struct {
	Events  *Events
	Workers *workerpool.Group
	error   *event.Event1[error]
	options *Options

	*APIProvider
	*NetworkManager
	*ChainManager
	*EngineManager

	log.Logger
	module.Module
}

func New(logger log.Logger, workers *workerpool.Group, dispatcher network.Endpoint, opts ...options.Option[Protocol]) *Protocol {
	return options.Apply(&Protocol{
		Events:  NewEvents(),
		Workers: workers,
		Logger:  logger,
		error:   event.New1[error](),
		options: newOptions(),
	}, opts, func(p *Protocol) {
		p.APIProvider = NewAPIProvider(p)
		p.ChainManager = newChainManager(p)
		p.EngineManager = NewEngineManager(p)
		p.NetworkManager = newNetwork(p, dispatcher)
	}, (*Protocol).TriggerConstructed)
}

func (p *Protocol) Run(ctx context.Context) error {
	p.TriggerInitialized()

	<-ctx.Done()

	p.TriggerShutdown()
	p.TriggerStopped()

	p.Workers.Shutdown()

	return ctx.Err()
}

func (p *Protocol) TriggerInitialized() {
	var waitInitialized sync.WaitGroup

	waitInitialized.Add(1)
	p.MainChain.OnUpdateOnce(func(_, mainChain *Chain) {
		mainChain.Engine.OnUpdateOnce(func(_, _ *engine.Engine) {
			p.Module.TriggerInitialized()

			waitInitialized.Done()
		})
	})

	waitInitialized.Wait()
}
