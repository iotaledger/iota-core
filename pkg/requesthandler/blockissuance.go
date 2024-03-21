package requesthandler

import (
	"context"
	"time"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/filter/postsolidfilter"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/filter/presolidfilter"
	"github.com/iotaledger/iota-core/pkg/retainer/txretainer"
	iotago "github.com/iotaledger/iota.go/v4"
)

// TODO: make sure an honest validator does not issue blocks within the same slot ratification period in two conflicting chains.
//  - this can be achieved by remembering the last issued block together with the engine name/chain.
//  - if the engine name/chain is the same we can always issue a block.
//  - if the engine name/chain is different we need to make sure to wait "slot ratification" slots.

// SubmitBlockWithoutAwaitingBooking submits a block to be processed.
func (r *RequestHandler) SubmitBlockWithoutAwaitingBooking(block *model.Block) error {
	return r.submitBlock(block)
}

// submitBlockAndAwaitRetainer submits a block to be processed and waits for the block gets retained.
func (r *RequestHandler) submitBlockAndAwaitRetainer(ctx context.Context, block *model.Block) error {
	filtered := make(chan error, 1)
	exit := make(chan struct{})
	defer close(exit)

	// Make sure we don't wait forever here. If the block is not dispatched to the main engine,
	// it will never trigger one of the below events.
	processingCtx, processingCtxCancel := context.WithTimeout(ctx, 10*time.Second)
	defer processingCtxCancel()
	// Calculate the blockID so that we don't capture the block pointer in the event handlers.
	blockID := block.ID()

	var successUnhook func()
	// Hook to TransactionAttached event if the block contains a transaction.
	signedTx, isTx := block.SignedTransaction()
	if isTx {
		txID := signedTx.Transaction.MustID()
		// Check if the transaction is already retained. The onTransactionAttached event is only triggered if it's a new transaction.
		// If the transaction is already retained, we hook to the BlockRetained event.
		_, err := r.protocol.Engines.Main.Get().TxRetainer.TransactionMetadata(txID)
		if ierrors.Is(err, txretainer.ErrEntryNotFound) {
			successUnhook = r.protocol.Events.Engine.TransactionRetainer.TransactionRetained.Hook(func(transactionID iotago.TransactionID) {
				if transactionID != txID {
					return
				}
				select {
				case filtered <- nil:
				case <-exit:
				}
			}, event.WithWorkerPool(r.workerPool)).Unhook
		}
	}

	// if no hook was set, hook to the block retained event.
	if successUnhook == nil {
		successUnhook = r.protocol.Events.Engine.BlockRetainer.BlockRetained.Hook(func(eventBlock *blocks.Block) {
			if blockID != eventBlock.ID() {
				return
			}
			select {
			case filtered <- nil:
			case <-exit:
			}
		}, event.WithWorkerPool(r.workerPool)).Unhook
	}

	prefilteredUnhook := r.protocol.Events.Engine.PreSolidFilter.BlockPreFiltered.Hook(func(event *presolidfilter.BlockPreFilteredEvent) {
		if blockID != event.Block.ID() {
			return
		}
		select {
		case filtered <- event.Reason:
		case <-exit:
		}
	}, event.WithWorkerPool(r.workerPool)).Unhook

	postfilteredUnhook := r.protocol.Events.Engine.PostSolidFilter.BlockFiltered.Hook(func(event *postsolidfilter.BlockFilteredEvent) {
		if blockID != event.Block.ID() {
			return
		}
		select {
		case filtered <- event.Reason:
		case <-exit:
		}
	}, event.WithWorkerPool(r.workerPool)).Unhook

	defer lo.BatchReverse(successUnhook, prefilteredUnhook, postfilteredUnhook)()

	if err := r.submitBlock(block); err != nil {
		return ierrors.Wrapf(err, "failed to issue block %s", blockID)
	}
	select {
	case <-processingCtx.Done():
		return ierrors.Errorf("context canceled whilst waiting for event on block %s", blockID)
	case err := <-filtered:
		if err != nil {
			return ierrors.Wrapf(err, "block filtered %s", blockID)
		}

		return nil
	}
}

func (r *RequestHandler) SubmitBlockAndAwaitRetainer(ctx context.Context, iotaBlock *iotago.Block) (iotago.BlockID, error) {
	modelBlock, err := model.BlockFromBlock(iotaBlock)
	if err != nil {
		return iotago.EmptyBlockID, ierrors.Wrap(err, "error serializing block to model block")
	}

	if err = r.submitBlockAndAwaitRetainer(ctx, modelBlock); err != nil {
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
