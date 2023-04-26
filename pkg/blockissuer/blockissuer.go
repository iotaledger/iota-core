package blockissuer

import (
	"context"
	"time"

	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/timeutil"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/builder"
)

// BlockIssuer contains logic to create and references block issuer.
type BlockIssuer struct {
	events *Events

	workerPool *workerpool.WorkerPool

	Account  Account
	protocol *protocol.Protocol

	optsTipSelectionTimeout       time.Duration
	optsTipSelectionRetryInterval time.Duration
}

func New(p *protocol.Protocol, account Account, opts ...options.Option[BlockIssuer]) *BlockIssuer {
	return options.Apply(&BlockIssuer{
		events:     NewEvents(),
		workerPool: p.Workers.CreatePool("BlockIssuer"),
		protocol:   p,
		Account:    account,
	}, opts)
}

// Shutdown shuts down the block issuer.
func (i *BlockIssuer) Shutdown() {
	i.workerPool.Shutdown()
	i.workerPool.ShutdownComplete.Wait()
}

// IssueBlock submits a block to be processed.
func (i *BlockIssuer) IssueBlock(block *model.Block) error {
	return i.issueBlock(block)
}

// IssueBlockAndAwaitEvent submits a block to be processed and waits for the event to be triggered.
func (i *BlockIssuer) IssueBlockAndAwaitEvent(ctx context.Context, block *model.Block, evt *event.Event1[*blocks.Block]) error {
	triggered := make(chan *blocks.Block, 1)
	exit := make(chan struct{})
	defer close(exit)

	defer evt.Hook(func(eventBlock *blocks.Block) {
		if block.ID() != eventBlock.ID() {
			return
		}
		select {
		case triggered <- eventBlock:
		case <-exit:
		}
	}, event.WithWorkerPool(i.workerPool)).Unhook()

	if err := i.issueBlock(block); err != nil {
		return errors.Wrapf(err, "failed to issue block %s", block.ID().String())
	}

	select {
	case <-ctx.Done():
		return errors.Errorf("context canceled whilst waiting for event on block %s", block.ID())
	case <-triggered:
		return nil
	}
}

// CreateBlock creates a new block including sequence number and tip selection and returns it.
func (i *BlockIssuer) CreateBlock(ctx context.Context, p iotago.Payload, parentsCount ...int) (*model.Block, error) {
	return i.CreateBlockWithReferences(ctx, p, nil, parentsCount...)
}

// CreateBlockWithReferences creates a new block with the references submit.
func (i *BlockIssuer) CreateBlockWithReferences(ctx context.Context, p iotago.Payload, references model.ParentReferences, strongParentsCountOpt ...int) (*model.Block, error) {
	strongParentsCount := iotago.BlockMaxParents
	if len(strongParentsCountOpt) > 0 {
		strongParentsCount = strongParentsCountOpt[0]
	}

	var err error
	if references == nil {
		references, err = i.getReferencesWithRetry(ctx, p, strongParentsCount)
		if err != nil {
			return nil, errors.Wrap(err, "error while trying to get references")
		}
	}

	slotCommitment := i.protocol.MainEngineInstance().Storage.Settings().LatestCommitment()
	lastFinalizedSlot := i.protocol.MainEngineInstance().Storage.Settings().LatestFinalizedSlot()

	// TODO: do we have to ensure that the issuing time is at least the Max (latest) of the parents' issuing times?
	block, err := builder.NewBlockBuilder().
		StrongParents(references[model.StrongParentType]).
		WeakParents(references[model.WeakParentType]).
		ShallowLikeParents(references[model.ShallowLikeParentType]).
		SlotCommitment(slotCommitment).
		LatestFinalizedSlot(lastFinalizedSlot).
		Payload(p).
		Sign(i.Account.ID(), i.Account.PrivateKey()).
		ProofOfWork(ctx, float64(i.protocol.MainEngineInstance().Storage.Settings().ProtocolParameters().MinPoWScore)).
		Build()
	if err != nil {
		return nil, errors.Wrap(err, "error building block")
	}

	modelBlock, err := model.BlockFromBlock(block, i.protocol.API())
	if err != nil {
		return nil, errors.Wrap(err, "error serializing block to model block")
	}

	i.events.BlockConstructed.Trigger(modelBlock)

	return modelBlock, nil
}

func (i *BlockIssuer) issueBlock(block *model.Block) error {
	if err := i.protocol.ProcessOwnBlock(block); err != nil {
		return err
	}
	i.events.BlockIssued.Trigger(block)

	return nil
}

// getReferencesWithRetry tries to get references for the given payload. If it fails, it will retry at regular intervals until
// the timeout is reached.
func (i *BlockIssuer) getReferencesWithRetry(ctx context.Context, _ iotago.Payload, parentsCount int) (references model.ParentReferences, err error) {
	timeout := time.NewTimer(i.optsTipSelectionTimeout)
	interval := time.NewTicker(i.optsTipSelectionRetryInterval)
	defer timeutil.CleanupTimer(timeout)
	defer timeutil.CleanupTicker(interval)

	for {
		references = i.protocol.TipManager.Tips(parentsCount)
		if len(references[model.StrongParentType]) > 0 {
			return references, nil
		}

		select {
		case <-interval.C:
			i.events.Error.Trigger(errors.Wrap(err, "could not get references"))
			continue
		case <-timeout.C:
			return nil, errors.Errorf("timeout while trying to select tips and determine references")
		case <-ctx.Done():
			return nil, errors.Errorf("context cancelled whilst trying to select tips and determine references: %s", ctx.Err().Error())
		}
	}
}

func WithTipSelectionTimeout(timeout time.Duration) options.Option[BlockIssuer] {
	return func(i *BlockIssuer) {
		i.optsTipSelectionTimeout = timeout
	}
}

func WithTipSelectionRetryInterval(interval time.Duration) options.Option[BlockIssuer] {
	return func(i *BlockIssuer) {
		i.optsTipSelectionRetryInterval = interval
	}
}
