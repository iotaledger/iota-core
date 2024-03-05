package requesthandler

import (
	"context"
	"time"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
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

// RequestHandler contains the logic to handle api requests.
type RequestHandler struct {
	workerPool *workerpool.WorkerPool

	protocol *protocol.Protocol
}

func New(p *protocol.Protocol) *RequestHandler {
	return &RequestHandler{
		workerPool: p.Workers.CreatePool("BlockHandler"),
		protocol:   p,
	}
}

// Shutdown shuts down the block issuer.
func (r *RequestHandler) Shutdown() {
	r.workerPool.Shutdown()
	r.workerPool.ShutdownComplete.Wait()
}

// SubmitBlock submits a block to be processed.
func (r *RequestHandler) SubmitBlock(block *model.Block) error {
	return r.submitBlock(block)
}

// SubmitBlockAndAwaitEvent submits a block to be processed and waits for the event to be triggered.
func (r *RequestHandler) SubmitBlockAndAwaitEvent(ctx context.Context, block *model.Block, evt *event.Event1[*blocks.Block]) error {
	triggered := make(chan error, 1)
	exit := make(chan struct{})
	defer close(exit)

	// Make sure we don't wait forever here. If the block is not dispatched to the main engine,
	// it will never trigger one of the below events.
	processingCtx, processingCtxCancel := context.WithTimeout(ctx, 5*time.Second)
	defer processingCtxCancel()

	// Calculate the blockID so that we don't capture the block pointer in the event handlers.
	blockID := block.ID()

	evtUnhook := evt.Hook(func(eventBlock *blocks.Block) {
		if blockID != eventBlock.ID() {
			return
		}
		select {
		case triggered <- nil:
		case <-exit:
		}
	}, event.WithWorkerPool(r.workerPool)).Unhook

	prefilteredUnhook := r.protocol.Events.Engine.PreSolidFilter.BlockPreFiltered.Hook(func(event *presolidfilter.BlockPreFilteredEvent) {
		if blockID != event.Block.ID() {
			return
		}
		select {
		case triggered <- event.Reason:
		case <-exit:
		}
	}, event.WithWorkerPool(r.workerPool)).Unhook

	defer lo.Batch(evtUnhook, prefilteredUnhook)()

	if err := r.submitBlock(block); err != nil {
		return ierrors.Wrapf(err, "failed to issue block %s", blockID)
	}

	select {
	case <-processingCtx.Done():
		return ierrors.Errorf("context canceled whilst waiting for event on block %s", blockID)
	case err := <-triggered:
		if err != nil {
			return ierrors.Wrapf(err, "block filtered out %s", blockID)
		}

		return nil
	}
}

func (r *RequestHandler) AttachBlock(ctx context.Context, iotaBlock *iotago.Block) (iotago.BlockID, error) {
	modelBlock, err := model.BlockFromBlock(iotaBlock)
	if err != nil {
		return iotago.EmptyBlockID, ierrors.Wrap(err, "error serializing block to model block")
	}

	if err = r.SubmitBlockAndAwaitEvent(ctx, modelBlock, r.protocol.Events.Engine.BlockDAG.BlockAttached); err != nil {
		return iotago.EmptyBlockID, ierrors.Wrap(err, "error issuing model block")
	}

	return modelBlock.ID(), nil
}

func (r *RequestHandler) submitBlock(block *model.Block) error {
	if err := r.protocol.IssueBlock(block); err != nil {
		return err
	}

	return nil
}