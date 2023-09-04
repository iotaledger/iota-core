package protocol

import (
	"context"

	"github.com/iotaledger/hive.go/crypto/identity"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/network"
)

type Protocol struct {
	Workers *workerpool.Group

	*Engines
	*Chains
	*Network

	error *event.Event1[error]

	Options *Options
}

func New(workers *workerpool.Group, dispatcher network.Endpoint, opts ...options.Option[Protocol]) *Protocol {
	return options.Apply(&Protocol{
		Workers: workers,
		Options: NewOptions(),
	}, opts, func(p *Protocol) {
		p.Engines = NewEngines(p)
		p.Chains = NewChains(p.MainEngine())

		p.Network.OnError(func(err error, src network.PeerID) { p.error.Trigger(ierrors.Wrapf(err, "Network error from %s", src)) })
	})
}

func (p *Protocol) Run(ctx context.Context) error {
	return nil
}

func (p *Protocol) OnError(callback func(error)) (unsubscribe func()) {
	return p.error.Hook(callback).Unhook
}

func (p *Protocol) IssueBlock(block *model.Block) error {
	p.MainEngine().ProcessBlockFromPeer(block, identity.ID{})

	return nil
}
