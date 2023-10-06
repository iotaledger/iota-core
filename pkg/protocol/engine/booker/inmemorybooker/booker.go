package inmemorybooker

import (
	"sync/atomic"

	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/booker"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledger"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/conflictdag"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/nodeclient/apimodels"
)

type Booker struct {
	events *booker.Events

	workers *workerpool.Group

	blockCache *blocks.Blocks

	conflictDAG conflictdag.ConflictDAG[iotago.TransactionID, mempool.StateID, ledger.BlockVoteRank]

	ledger ledger.Ledger

	retainBlockFailure func(id iotago.BlockID, reason apimodels.BlockFailureReason)

	errorHandler func(error)
	apiProvider  iotago.APIProvider

	module.Module
}

func NewProvider(opts ...options.Option[Booker]) module.Provider[*engine.Engine, booker.Booker] {
	return module.Provide(func(e *engine.Engine) booker.Booker {
		b := New(e.Workers.CreateGroup("Booker"), e, e.BlockCache, e.ErrorHandler("booker"), opts...)
		e.HookConstructed(func() {
			b.ledger = e.Ledger
			b.ledger.HookConstructed(func() {
				b.conflictDAG = b.ledger.ConflictDAG()

				b.ledger.MemPool().OnTransactionAttached(func(transaction mempool.TransactionMetadata) {
					transaction.OnInvalid(func(err error) {
						b.events.TransactionInvalid.Trigger(transaction, err)
					})
				})
			})

			e.Events.SeatManager.BlockProcessed.Hook(func(block *blocks.Block) {
				if err := b.Queue(block); err != nil {
					b.errorHandler(err)
				}
			})

			b.setRetainBlockFailureFunc(e.Retainer.RetainBlockFailure)

			e.Events.Booker.LinkTo(b.events)

			b.TriggerInitialized()
		})

		return b
	})
}

func New(workers *workerpool.Group, apiProvider iotago.APIProvider, blockCache *blocks.Blocks, errorHandler func(error), opts ...options.Option[Booker]) *Booker {
	return options.Apply(&Booker{
		events:      booker.NewEvents(),
		apiProvider: apiProvider,

		blockCache:   blockCache,
		workers:      workers,
		errorHandler: errorHandler,
	}, opts, (*Booker).TriggerConstructed)
}

var _ booker.Booker = new(Booker)

// Queue checks if payload is solid and then sets up the block to react to its parents.
func (b *Booker) Queue(block *blocks.Block) error {
	signedTransactionMetadata, containsTransaction := b.ledger.AttachTransaction(block)

	if !containsTransaction {
		b.setupBlock(block)
		return nil
	}

	if signedTransactionMetadata == nil {
		b.retainBlockFailure(block.ID(), apimodels.BlockFailurePayloadInvalid)

		return ierrors.Errorf("transaction in %s was not attached", block.ID())
	}

	// Based on the assumption that we always fork and the UTXO and Tangle past cones are always fully known.
	signedTransactionMetadata.OnSignaturesValid(func() {
		transactionMetadata := signedTransactionMetadata.TransactionMetadata()

		if orphanedSlot, isOrphaned := transactionMetadata.OrphanedSlot(); isOrphaned && orphanedSlot <= block.SlotCommitmentID().Slot() {
			block.SetInvalid()

			return
		}

		transactionMetadata.OnBooked(func() {
			block.SetPayloadConflictIDs(transactionMetadata.ConflictIDs())
			b.setupBlock(block)
		})
	})

	return nil
}

func (b *Booker) Shutdown() {
	b.TriggerStopped()
	b.workers.Shutdown()
}

func (b *Booker) setupBlock(block *blocks.Block) {
	var unbookedParentsCount atomic.Int32
	unbookedParentsCount.Store(int32(len(block.Parents())))

	block.ForEachParent(func(parent iotago.Parent) {
		parentBlock, exists := b.blockCache.Block(parent.ID)
		if !exists {
			panic("cannot setup block without existing parent")
		}

		parentBlock.Booked().OnUpdateOnce(func(_, _ bool) {
			if unbookedParentsCount.Add(-1) == 0 {
				if err := b.book(block); err != nil {
					if block.SetInvalid() {
						b.events.BlockInvalid.Trigger(block, ierrors.Wrap(err, "failed to book block"))
					}
				}
			}
		})

		parentBlock.Invalid().OnUpdateOnce(func(_, _ bool) {
			if block.SetInvalid() {
				b.events.BlockInvalid.Trigger(block, ierrors.New("block marked as invalid in Booker"))
			}
		})
	})
}

func (b *Booker) setRetainBlockFailureFunc(retainBlockFailure func(iotago.BlockID, apimodels.BlockFailureReason)) {
	b.retainBlockFailure = retainBlockFailure
}

func (b *Booker) book(block *blocks.Block) error {
	conflictsToInherit, err := b.inheritConflicts(block)
	if err != nil {
		return ierrors.Wrapf(err, "failed to inherit conflicts for block %s", block.ID())
	}

	// The block does not inherit conflicts that have been orphaned with respect to its commitment.
	for it := conflictsToInherit.Iterator(); it.HasNext(); {
		conflictID := it.Next()

		txMetadata, exists := b.ledger.MemPool().TransactionMetadata(conflictID)
		if !exists {
			return ierrors.Errorf("failed to load transaction %s for block %s", conflictID.String(), block.ID())
		}

		if orphanedSlot, orphaned := txMetadata.OrphanedSlot(); orphaned && orphanedSlot <= block.SlotCommitmentID().Slot() {
			// Merge-to-master orphaned conflicts.
			conflictsToInherit.Delete(conflictID)
		}
	}

	block.SetConflictIDs(conflictsToInherit)
	block.SetBooked()
	b.events.BlockBooked.Trigger(block)

	return nil
}

func (b *Booker) inheritConflicts(block *blocks.Block) (conflictIDs ds.Set[iotago.TransactionID], err error) {
	conflictIDsToInherit := ds.NewSet[iotago.TransactionID]()

	// Inherit conflictIDs from parents based on the parent type.
	for _, parent := range block.ParentsWithType() {
		parentBlock, exists := b.blockCache.Block(parent.ID)
		if !exists {
			b.retainBlockFailure(block.ID(), apimodels.BlockFailureParentNotFound)
			return nil, ierrors.Errorf("parent %s does not exist", parent.ID)
		}

		switch parent.Type {
		case iotago.StrongParentType:
			conflictIDsToInherit.AddAll(parentBlock.ConflictIDs())
		case iotago.WeakParentType:
			conflictIDsToInherit.AddAll(parentBlock.PayloadConflictIDs())
		case iotago.ShallowLikeParentType:
			// Check whether the parent contains a conflicting TX,
			// otherwise reference is invalid and the block should be marked as invalid as well.
			if signedTransaction, hasTx := parentBlock.SignedTransaction(); !hasTx || !parentBlock.PayloadConflictIDs().Has(lo.PanicOnErr(signedTransaction.Transaction.ID())) {
				return nil, ierrors.Wrapf(err, "shallow like parent %s does not contain a conflicting transaction", parent.ID.String())
			}

			conflictIDsToInherit.AddAll(parentBlock.PayloadConflictIDs())
			//  remove all conflicting conflicts from conflictIDsToInherit
			for _, conflictID := range parentBlock.PayloadConflictIDs().ToSlice() {
				if conflictingConflicts, exists := b.conflictDAG.ConflictingConflicts(conflictID); exists {
					conflictIDsToInherit.DeleteAll(b.conflictDAG.FutureCone(conflictingConflicts))
				}
			}
		}
	}

	// Add all conflicts from the block's payload itself.
	// Forking on booking: we determine the block's PayloadConflictIDs by treating each TX as a conflict.
	conflictIDsToInherit.AddAll(block.PayloadConflictIDs())

	// Only inherit conflicts that are not yet accepted (aka merge to master).
	return b.conflictDAG.UnacceptedConflicts(conflictIDsToInherit), nil
}
