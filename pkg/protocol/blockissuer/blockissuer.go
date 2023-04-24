package blockissuer

import (
	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/crypto/ed25519"
	"github.com/iotaledger/hive.go/crypto/identity"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/network"
	"github.com/iotaledger/iota-core/pkg/protocol/blockissuer/blockfactory"
	"github.com/iotaledger/iota-core/pkg/protocol/tipmanager"
	iotago "github.com/iotaledger/iota.go/v4"
)

// region Factory ///////////////////////////////////////////////////////////////////////////////////////////////

// BlockIssuer contains logic to create and issue blocks.
type BlockIssuer struct {
	Events *Events

	factory *blockfactory.Factory

	localPrivateKey      ed25519.PrivateKey
	isBootstrappedFunc   func() bool
	latestCommitmentFunc blockfactory.CommitmentFunc
	processBlockFunc     func(*model.Block, network.PeerID) error

	optsBlockFactoryOptions []options.Option[blockfactory.Factory]
}

// New creates a new block issuer.
func New(localPrivateKey ed25519.PrivateKey, isBootstrappedFunc func() bool, latestCommitmentFunc blockfactory.CommitmentFunc, processBlockFunc func(*model.Block, network.PeerID) error, tipManager tipmanager.TipManager, opts ...options.Option[BlockIssuer]) *BlockIssuer {
	return options.Apply(&BlockIssuer{
		Events:               NewEvents(),
		localPrivateKey:      localPrivateKey,
		isBootstrappedFunc:   isBootstrappedFunc,
		latestCommitmentFunc: latestCommitmentFunc,
		processBlockFunc:     processBlockFunc,
	}, opts, func(i *BlockIssuer) {
		i.factory = blockfactory.NewBlockFactory(
			localPrivateKey,
			tipManager,
			// TODO: change when we have a way to fix the opinion coming from strong parents.
			func(_ iotago.Payload, strongParents iotago.BlockIDs) (model.ParentReferences, error) {
				return model.ParentReferences{
					model.StrongParentType: strongParents,
				}, nil
			},
			latestCommitmentFunc,
			i.optsBlockFactoryOptions...)

		i.Events.BlockConstructed.LinkTo(i.factory.Events.BlockConstructed)
		i.Events.Error.LinkTo(i.factory.Events.Error)
	})
}

// IssuePayload creates a new block including sequence number and tip selection, submits it to be processed and returns it.
func (i *BlockIssuer) IssuePayload(p iotago.Payload, parentsCount ...int) (block *model.Block, err error) {
	block, err = i.factory.CreateBlock(p, parentsCount...)
	if err != nil {
		i.Events.Error.Trigger(errors.Wrap(err, "block could not be created"))
		return block, err
	}
	return block, i.issueBlock(block)
}

// IssuePayloadWithReferences creates a new block with the references submit.
func (i *BlockIssuer) IssuePayloadWithReferences(p iotago.Payload, references model.ParentReferences, strongParentsCountOpt ...int) (block *model.Block, err error) {
	block, err = i.factory.CreateBlockWithReferences(p, references, strongParentsCountOpt...)
	if err != nil {
		i.Events.Error.Trigger(errors.Wrap(err, "block with references could not be created"))
		return nil, err
	}

	return block, i.issueBlock(block)
}

func (i *BlockIssuer) issueBlock(block *model.Block) error {
	if err := i.processBlockFunc(block, identity.NewID(i.localPrivateKey.Public())); err != nil {
		return err
	}
	i.Events.BlockIssued.Trigger(block)

	return nil
}

func WithBlockFactoryOptions(blockFactoryOptions ...options.Option[blockfactory.Factory]) options.Option[BlockIssuer] {
	return func(issuer *BlockIssuer) {
		issuer.optsBlockFactoryOptions = blockFactoryOptions
	}
}
