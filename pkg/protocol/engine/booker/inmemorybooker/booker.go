package inmemorybooker

import (
	"sync/atomic"

	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/booker"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledger"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/spenddag"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/nodeclient/apimodels"
)

type Booker struct {
	events *booker.Events

	blockCache *blocks.Blocks

	spendDAG spenddag.SpendDAG[iotago.TransactionID, mempool.StateID, ledger.BlockVoteRank]

	ledger ledger.Ledger

	retainBlockFailure func(id iotago.BlockID, reason apimodels.BlockFailureReason)

	errorHandler func(error)
	apiProvider  iotago.APIProvider

	module.Module
}

func NewProvider(opts ...options.Option[Booker]) module.Provider[*engine.Engine, booker.Booker] {
	return module.Provide(func(e *engine.Engine) booker.Booker {
		b := New(e, e.BlockCache, e.ErrorHandler("booker"), opts...)
		e.HookConstructed(func() {
			b.ledger = e.Ledger
			b.ledger.HookConstructed(func() {
				b.spendDAG = b.ledger.SpendDAG()

				b.ledger.MemPool().OnTransactionAttached(func(transaction mempool.TransactionMetadata) {
					transaction.OnAccepted(func() {
						b.events.TransactionAccepted.Trigger(transaction)
					})
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

func New(apiProvider iotago.APIProvider, blockCache *blocks.Blocks, errorHandler func(error), opts ...options.Option[Booker]) *Booker {
	return options.Apply(&Booker{
		events:      booker.NewEvents(),
		apiProvider: apiProvider,

		blockCache:   blockCache,
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
			block.SetPayloadSpendIDs(transactionMetadata.SpendIDs())
			b.setupBlock(block)
		})
	})

	return nil
}

func (b *Booker) Shutdown() {
	b.TriggerStopped()
}

func (b *Booker) setupBlock(block *blocks.Block) {
	var unbookedParentsCount atomic.Int32
	unbookedParentsCount.Store(int32(len(block.Parents())))

	block.ForEachParent(func(parent iotago.Parent) {
		parentBlock, exists := b.blockCache.Block(parent.ID)
		if !exists {
			b.errorHandler(ierrors.Errorf("cannot setup block %s without existing parent %s", block.ID(), parent.ID))

			return
		}

		parentBlock.Booked().OnUpdateOnce(func(_ bool, _ bool) {
			if unbookedParentsCount.Add(-1) == 0 {
				if err := b.book(block); err != nil {
					if block.SetInvalid() {
						b.events.BlockInvalid.Trigger(block, ierrors.Wrap(err, "failed to book block"))
					}
				}
			}
		})

		parentBlock.Invalid().OnUpdateOnce(func(_ bool, _ bool) {
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
	spendsToInherit, err := b.inheritSpends(block)
	if err != nil {
		return ierrors.Wrapf(err, "failed to inherit spends for block %s", block.ID())
	}

	// The block does not inherit spends that have been orphaned with respect to its commitment.
	for it := spendsToInherit.Iterator(); it.HasNext(); {
		spendID := it.Next()

		txMetadata, exists := b.ledger.MemPool().TransactionMetadata(spendID)
		if !exists {
			return ierrors.Errorf("failed to load transaction %s for block %s", spendID.String(), block.ID())
		}

		if orphanedSlot, orphaned := txMetadata.OrphanedSlot(); orphaned && orphanedSlot <= block.SlotCommitmentID().Slot() {
			// Merge-to-master orphaned spends.
			spendsToInherit.Delete(spendID)
		}
	}

	block.SetSpendIDs(spendsToInherit)
	block.SetBooked()
	b.events.BlockBooked.Trigger(block)

	return nil
}

func (b *Booker) inheritSpends(block *blocks.Block) (spendIDs ds.Set[iotago.TransactionID], err error) {
	spendIDsToInherit := ds.NewSet[iotago.TransactionID]()

	// Inherit spendIDs from parents based on the parent type.
	for _, parent := range block.ParentsWithType() {
		parentBlock, exists := b.blockCache.Block(parent.ID)
		if !exists {
			b.retainBlockFailure(block.ID(), apimodels.BlockFailureParentNotFound)
			return nil, ierrors.Errorf("parent %s does not exist", parent.ID)
		}

		switch parent.Type {
		case iotago.StrongParentType:
			spendIDsToInherit.AddAll(parentBlock.SpendIDs())
		case iotago.WeakParentType:
			spendIDsToInherit.AddAll(parentBlock.PayloadSpendIDs())
		case iotago.ShallowLikeParentType:
			// Check whether the parent contains a conflicting TX,
			// otherwise reference is invalid and the block should be marked as invalid as well.
			if signedTransaction, hasTx := parentBlock.SignedTransaction(); !hasTx || !parentBlock.PayloadSpendIDs().Has(lo.PanicOnErr(signedTransaction.Transaction.ID())) {
				return nil, ierrors.Wrapf(err, "shallow like parent %s does not contain a conflicting transaction", parent.ID.String())
			}

			spendIDsToInherit.AddAll(parentBlock.PayloadSpendIDs())
			//  remove all conflicting spends from spendIDsToInherit
			for _, spendID := range parentBlock.PayloadSpendIDs().ToSlice() {
				if conflictingSpends, exists := b.spendDAG.ConflictingSpends(spendID); exists {
					spendIDsToInherit.DeleteAll(b.spendDAG.FutureCone(conflictingSpends))
				}
			}
		}
	}

	// Add all spends from the block's payload itself.
	// Forking on booking: we determine the block's PayloadSpendIDs by treating each TX as a conflict.
	spendIDsToInherit.AddAll(block.PayloadSpendIDs())

	// Only inherit spends that are not yet accepted (aka merge to master).
	return b.spendDAG.UnacceptedSpends(spendIDsToInherit), nil
}
