package blockissuer

import (
	"time"

	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/crypto/ed25519"
	"github.com/iotaledger/hive.go/crypto/identity"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/protocol/blockissuer/blockfactory"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	iotago "github.com/iotaledger/iota.go/v4"
)

// region Factory ///////////////////////////////////////////////////////////////////////////////////////////////

// BlockIssuer contains logic to create and issue blocks.
type BlockIssuer struct {
	Events *Events

	*blockfactory.Factory
	protocol        *protocol.Protocol
	localPrivateKey ed25519.PrivateKey
	workerPool      *workerpool.WorkerPool

	optsBlockFactoryOptions    []options.Option[blockfactory.Factory]
	optsIgnoreBootstrappedFlag bool
}

// New creates a new block issuer.
func New(protocol *protocol.Protocol, localPrivateKey ed25519.PrivateKey, opts ...options.Option[BlockIssuer]) *BlockIssuer {
	return options.Apply(&BlockIssuer{
		Events:          NewEvents(),
		localPrivateKey: localPrivateKey,
		protocol:        protocol,
		workerPool:      protocol.Workers.CreatePool("BlockIssuer", 2),
	}, opts, func(i *BlockIssuer) {
		i.Factory = blockfactory.NewBlockFactory(
			localPrivateKey,
			i.protocol.TipManager,
			// TODO: change when we have a way to fix the opinion coming from strong parents.
			func(_ iotago.Payload, strongParents iotago.BlockIDs) (model.ParentReferences, error) {
				return model.ParentReferences{
					model.StrongParentType: strongParents,
				}, nil
			},
			func() (ecRecord *iotago.Commitment, lastFinalizedSlot iotago.SlotIndex, err error) {
				latestCommitment := i.protocol.MainEngineInstance().Storage.Settings().LatestCommitment()
				confirmedSlotIndex := i.protocol.MainEngineInstance().Storage.Settings().LatestFinalizedSlot()

				return latestCommitment, confirmedSlotIndex, nil
			},
			i.optsBlockFactoryOptions...)
	}, (*BlockIssuer).setupEvents)
}

func (i *BlockIssuer) setupEvents() {
	i.Events.Error.LinkTo(i.Factory.Events.Error)
}

// IssuePayload creates a new block including sequence number and tip selection, submits it to be processed and returns it.
func (i *BlockIssuer) IssuePayload(p iotago.Payload, parentsCount ...int) (block *model.Block, err error) {
	if !i.optsIgnoreBootstrappedFlag && !i.protocol.MainEngineInstance().IsBootstrapped() {
		return nil, ErrNotBootstraped
	}

	block, err = i.Factory.CreateBlock(p, parentsCount...)
	if err != nil {
		i.Events.Error.Trigger(errors.Wrap(err, "block could not be created"))
		return block, err
	}
	return block, i.issueBlock(block)
}

// IssuePayloadWithReferences creates a new block with the references submit.
func (i *BlockIssuer) IssuePayloadWithReferences(p iotago.Payload, references model.ParentReferences, strongParentsCountOpt ...int) (block *model.Block, err error) {
	if !i.optsIgnoreBootstrappedFlag && !i.protocol.MainEngineInstance().IsBootstrapped() {
		return nil, ErrNotBootstraped
	}

	block, err = i.Factory.CreateBlockWithReferences(p, references, strongParentsCountOpt...)
	if err != nil {
		i.Events.Error.Trigger(errors.Wrap(err, "block with references could not be created"))
		return nil, err
	}

	return block, i.issueBlock(block)
}

func (i *BlockIssuer) issueBlock(block *model.Block) error {
	if err := i.protocol.ProcessBlock(block, identity.NewID(i.localPrivateKey.Public())); err != nil {
		return err
	}
	i.Events.BlockIssued.Trigger(block)

	return nil
}

// IssueBlockAndAwaitBlockToBeBooked awaits maxAwait for the given block to get booked.
func (i *BlockIssuer) IssueBlockAndAwaitBlockToBeBooked(block *model.Block, maxAwait time.Duration) error {
	if !i.optsIgnoreBootstrappedFlag && !i.protocol.MainEngineInstance().IsBootstrapped() {
		return ErrNotBootstraped
	}

	// first subscribe to the transaction booked event
	booked := make(chan *blocks.Block, 1)
	// exit is used to let the caller exit if for whatever
	// reason the same transaction gets booked multiple times
	exit := make(chan struct{})
	defer close(exit)

	defer i.protocol.Events.Engine.Booker.BlockBooked.Hook(func(bookedBlock *blocks.Block) {
		if bookedBlock.ID() != block.ID() {
			return
		}
		select {
		case booked <- bookedBlock:
		case <-exit:
		}
	}, event.WithWorkerPool(i.workerPool)).Unhook()

	if err := i.issueBlock(block); err != nil {
		return errors.Wrapf(err, "failed to issue block %s", block.ID().String())
	}

	timer := time.NewTimer(maxAwait)
	defer timer.Stop()
	select {
	case <-timer.C:
		return ErrBlockWasNotBookedInTime
	case <-booked:
		return nil
	}
}

// IssueBlockAndAwaitBlockToBeTracked awaits maxAwait for the given block to get tracked.
func (i *BlockIssuer) IssueBlockAndAwaitBlockToBeTracked(block *model.Block, maxAwait time.Duration) error {
	if !i.optsIgnoreBootstrappedFlag && !i.protocol.MainEngineInstance().IsBootstrapped() {
		return ErrNotBootstraped
	}

	// first subscribe to the transaction booked event
	booked := make(chan *blocks.Block, 1)
	// exit is used to let the caller exit if for whatever
	// reason the same transaction gets booked multiple times
	exit := make(chan struct{})
	defer close(exit)

	defer i.protocol.Events.Engine.Booker.WitnessAdded.Hook(func(trackedBlock *blocks.Block) {
		if trackedBlock.ID() != block.ID() {
			return
		}
		select {
		case booked <- trackedBlock:
		case <-exit:
		}
	}, event.WithWorkerPool(i.workerPool)).Unhook()

	if err := i.issueBlock(block); err != nil {
		return errors.Wrapf(err, "failed to issue block %s", block.ID().String())
	}

	timer := time.NewTimer(maxAwait)
	defer timer.Stop()
	select {
	case <-timer.C:
		return ErrBlockWasNotBookedInTime
	case <-booked:
		return nil
	}
}

func WithBlockFactoryOptions(blockFactoryOptions ...options.Option[blockfactory.Factory]) options.Option[BlockIssuer] {
	return func(issuer *BlockIssuer) {
		issuer.optsBlockFactoryOptions = blockFactoryOptions
	}
}

func WithIgnoreBootstrappedFlag(ignoreBootstrappedFlag bool) options.Option[BlockIssuer] {
	return func(issuer *BlockIssuer) {
		issuer.optsIgnoreBootstrappedFlag = ignoreBootstrappedFlag
	}
}
