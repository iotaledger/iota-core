package inmemorybooker

import (
	"github.com/iotaledger/hive.go/core/causalorder"
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
	"github.com/iotaledger/iota.go/v4/api"
	"github.com/iotaledger/iota.go/v4/nodeclient/apimodels"
)

type Booker struct {
	events *booker.Events

	bookingOrder *causalorder.CausalOrder[iotago.SlotIndex, iotago.BlockID, *blocks.Block]

	workers *workerpool.Group

	blockCache *blocks.Blocks

	conflictDAG conflictdag.ConflictDAG[iotago.TransactionID, iotago.OutputID, ledger.BlockVoteRank]

	ledger ledger.Ledger

	retainBlockFailure func(id iotago.BlockID, reason apimodels.BlockFailureReason)

	errorHandler func(error)
	apiProvider  api.Provider

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

func New(workers *workerpool.Group, apiProvider api.Provider, blockCache *blocks.Blocks, errorHandler func(error), opts ...options.Option[Booker]) *Booker {
	return options.Apply(&Booker{
		events:      booker.NewEvents(),
		apiProvider: apiProvider,

		blockCache:   blockCache,
		workers:      workers,
		errorHandler: errorHandler,
	}, opts, func(b *Booker) {
		b.bookingOrder = causalorder.New(
			workers.CreatePool("BookingOrder", 2),
			blockCache.Block,
			(*blocks.Block).IsBooked,
			b.book,
			b.markInvalid,
			(*blocks.Block).Parents,
			causalorder.WithReferenceValidator[iotago.SlotIndex, iotago.BlockID](b.isReferenceValid),
		)

		blockCache.Evict.Hook(b.evict)
	}, (*Booker).TriggerConstructed)
}

var _ booker.Booker = new(Booker)

// Queue checks if payload is solid and then adds the block to a Booker's CausalOrder.
func (b *Booker) Queue(block *blocks.Block) error {
	transactionMetadata, containsTransaction := b.ledger.AttachTransaction(block)

	if !containsTransaction {
		b.bookingOrder.Queue(block)
		return nil
	}

	if transactionMetadata == nil {
		b.retainBlockFailure(block.ID(), apimodels.BlockFailurePayloadInvalid)

		return ierrors.Errorf("transaction in %s was not attached", block.ID())
	}

	// Based on the assumption that we always fork and the UTXO and Tangle past cones are always fully known.
	transactionMetadata.OnBooked(func() {
		block.SetPayloadConflictIDs(transactionMetadata.ConflictIDs())
		b.bookingOrder.Queue(block)
	})

	return nil
}

func (b *Booker) Shutdown() {
	b.TriggerStopped()
	b.workers.Shutdown()
}

func (b *Booker) setRetainBlockFailureFunc(retainBlockFailure func(iotago.BlockID, apimodels.BlockFailureReason)) {
	b.retainBlockFailure = retainBlockFailure
}

func (b *Booker) evict(slotIndex iotago.SlotIndex) {
	b.bookingOrder.EvictUntil(slotIndex)
}

func (b *Booker) book(block *blocks.Block) error {
	conflictsToInherit, err := b.inheritConflicts(block)
	if err != nil {
		return ierrors.Wrapf(err, "failed to inherit conflicts for block %s", block.ID())
	}

	block.SetConflictIDs(conflictsToInherit)
	block.SetBooked()
	b.events.BlockBooked.Trigger(block)

	return nil
}

func (b *Booker) markInvalid(block *blocks.Block, err error) {
	if block.SetInvalid() {
		b.events.BlockInvalid.Trigger(block, ierrors.Wrap(err, "block marked as invalid in Booker"))
	}
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
			if tx, hasTx := parentBlock.Transaction(); !hasTx || !parentBlock.PayloadConflictIDs().Has(lo.PanicOnErr(tx.ID(b.apiProvider.APIForSlot(parentBlock.ID().Index())))) {
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

// isReferenceValid checks if the reference between the child and its parent is valid.
func (b *Booker) isReferenceValid(child *blocks.Block, parent *blocks.Block) (err error) {
	if parent.IsInvalid() {
		b.retainBlockFailure(child.ID(), apimodels.BlockFailureParentInvalid)
		return ierrors.Errorf("parent %s of child %s is marked as invalid", parent.ID(), child.ID())
	}

	return nil
}
