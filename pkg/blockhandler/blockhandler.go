package blockhandler

import (
	"context"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/filter/presolidfilter"
	iotago "github.com/iotaledger/iota.go/v4"
)

var (
	ErrBlockAttacherInvalidBlock              = ierrors.New("invalid block")
	ErrBlockAttacherAttachingNotPossible      = ierrors.New("attaching not possible")
	ErrBlockAttacherIncompleteBlockNotAllowed = ierrors.New("incomplete block is not allowed on this node")
)

// TODO: make sure an honest validator does not issue blocks within the same slot ratification period in two conflicting chains.
//  - this can be achieved by remembering the last issued block together with the engine name/chain.
//  - if the engine name/chain is the same we can always issue a block.
//  - if the engine name/chain is different we need to make sure to wait "slot ratification" slots.

// BlockIssuer contains logic to create and issue blocks signed by the given account.
type BlockHandler struct {
	events *Events

	workerPool *workerpool.WorkerPool

	protocol *protocol.Protocol
}

func New(p *protocol.Protocol) *BlockHandler {
	return &BlockHandler{
		events:     NewEvents(),
		workerPool: p.Workers.CreatePool("BlockIssuer"),
		protocol:   p,
	}
}

// Shutdown shuts down the block issuer.
func (i *BlockHandler) Shutdown() {
	i.workerPool.Shutdown()
	i.workerPool.ShutdownComplete.Wait()
}

// SubmitBlock submits a block to be processed.
func (i *BlockHandler) SubmitBlock(block *model.Block) error {
	return i.submitBlock(block)
}

// SubmitBlockAndAwaitEvent submits a block to be processed and waits for the event to be triggered.
func (i *BlockHandler) SubmitBlockAndAwaitEvent(ctx context.Context, block *model.Block, evt *event.Event1[*blocks.Block]) error {
	triggered := make(chan error, 1)
	exit := make(chan struct{})
	defer close(exit)

	defer evt.Hook(func(eventBlock *blocks.Block) {
		if block.ID() != eventBlock.ID() {
			return
		}
		select {
		case triggered <- nil:
		case <-exit:
		}
	}, event.WithWorkerPool(i.workerPool)).Unhook()

	defer i.protocol.Events.Engine.Filter.BlockPreFiltered.Hook(func(event *presolidfilter.BlockPreFilteredEvent) {
		if block.ID() != event.Block.ID() {
			return
		}
		select {
		case triggered <- event.Reason:
		case <-exit:
		}
	}, event.WithWorkerPool(i.workerPool)).Unhook()

	if err := i.submitBlock(block); err != nil {
		return ierrors.Wrapf(err, "failed to issue block %s", block.ID())
	}

	select {
	case <-ctx.Done():
		return ierrors.Errorf("context canceled whilst waiting for event on block %s", block.ID())
	case err := <-triggered:
		if err != nil {
			return ierrors.Wrapf(err, "block filtered out %s", block.ID())
		}

		return nil
	}
}

func (i *BlockHandler) AttachBlock(ctx context.Context, iotaBlock *iotago.Block) (iotago.BlockID, error) {
	modelBlock, err := model.BlockFromBlock(iotaBlock)
	if err != nil {
		return iotago.EmptyBlockID, ierrors.Wrap(err, "error serializing block to model block")
	}

	if err = i.SubmitBlockAndAwaitEvent(ctx, modelBlock, i.protocol.Events.Engine.BlockDAG.BlockAttached); err != nil {
		return iotago.EmptyBlockID, ierrors.Wrap(err, "error issuing model block")
	}

	return modelBlock.ID(), nil
}

func (i *BlockHandler) submitBlock(block *model.Block) error {
	if err := i.protocol.IssueBlock(block); err != nil {
		return err
	}

	i.events.BlockSubmitted.Trigger(block)

	return nil
}
