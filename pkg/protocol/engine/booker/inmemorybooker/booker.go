package inmemorybooker

import (
	"sync/atomic"

	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/booker"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledger"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/spenddag"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Booker struct {
	events *booker.Events

	blockCache *blocks.Blocks

	ledger ledger.Ledger

	spendDAG spenddag.SpendDAG[iotago.TransactionID, mempool.StateID, ledger.BlockVoteRank]

	loadBlockFromStorage func(id iotago.BlockID) (*model.Block, bool)

	errorHandler func(error)
	apiProvider  iotago.APIProvider

	module.Module
}

func NewProvider(opts ...options.Option[Booker]) module.Provider[*engine.Engine, booker.Booker] {
	return module.Provide(func(e *engine.Engine) booker.Booker {
		b := New(e, e.BlockCache, e.ErrorHandler("booker"), opts...)
		e.Constructed.OnTrigger(func() {
			b.ledger = e.Ledger
			b.ledger.HookConstructed(func() {
				b.spendDAG = b.ledger.SpendDAG()
				b.loadBlockFromStorage = e.Block
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
		return ierrors.Errorf("transaction in %s was not attached", block.ID())
	}

	// Based on the assumption that we always fork and the UTXO and Tangle past cones are always fully known.
	signedTransactionMetadata.OnSignaturesValid(func() {
		transactionMetadata := signedTransactionMetadata.TransactionMetadata()

		// If the transaction has been rejected and is orphaned, then it should stay orphaned despite new attachment coming in.
		// The new attachment should be marked as objectively invalid, because it's reattaching something that is rejected,
		// while committing to a slot in which the conflict was resolved.
		// If the transaction is pending, then it should be marked not orphaned anymore by the mem-pool.
		if orphanedSlot, isOrphaned := transactionMetadata.OrphanedSlot(); isOrphaned && transactionMetadata.IsRejected() && orphanedSlot <= block.SlotCommitmentID().Slot() {
			block.SetInvalid()

			return
		}

		transactionMetadata.OnBooked(func() {
			block.SetPayloadSpenderIDs(transactionMetadata.SpenderIDs())
			b.setupBlock(block)
		})

		transactionMetadata.OnInvalid(func(_ error) {
			b.setupBlock(block)
		})
	})

	signedTransactionMetadata.OnSignaturesInvalid(func(_ error) {
		b.setupBlock(block)
	})

	return nil
}

// Reset resets the component to a clean state as if it was created at the last commitment.
func (b *Booker) Reset() { /* nothing to reset but comply with interface */ }

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
				b.events.BlockInvalid.Trigger(block, ierrors.Errorf("block marked as invalid in Booker because parent block is invalid %s", parentBlock.ID()))
			}
		})
	})
}

func (b *Booker) book(block *blocks.Block) error {
	spendersToInherit, err := b.inheritSpenders(block)
	if err != nil {
		return ierrors.Wrapf(err, "failed to inherit spenders for block %s", block.ID())
	}

	// The block does not inherit spenders that have been orphaned with respect to its commitment.
	for it := spendersToInherit.Iterator(); it.HasNext(); {
		spenderID := it.Next()

		txMetadata, exists := b.ledger.MemPool().TransactionMetadata(spenderID)
		if !exists {
			return ierrors.Errorf("failed to load transaction %s for block %s", spenderID.String(), block.ID())
		}

		if orphanedSlot, orphaned := txMetadata.OrphanedSlot(); orphaned && orphanedSlot <= block.SlotCommitmentID().Slot() {
			// Merge-to-master orphaned spenders.
			spendersToInherit.Delete(spenderID)
		}
	}

	block.SetSpenderIDs(spendersToInherit)
	block.SetBooked()
	b.events.BlockBooked.Trigger(block)

	return nil
}

func (b *Booker) inheritSpenders(block *blocks.Block) (spenderIDs ds.Set[iotago.TransactionID], err error) {
	spenderIDsToInherit := ds.NewSet[iotago.TransactionID]()

	// Inherit spenderIDs from parents based on the parent type.
	for _, parent := range block.ParentsWithType() {
		parentBlock, exists := b.blockCache.Block(parent.ID)
		if !exists {
			return nil, ierrors.Errorf("parent %s does not exist", parent.ID)
		}

		switch parent.Type {
		case iotago.StrongParentType:
			spenderIDsToInherit.AddAll(parentBlock.SpenderIDs())
		case iotago.WeakParentType:
			spenderIDsToInherit.AddAll(parentBlock.PayloadSpenderIDs())
		case iotago.ShallowLikeParentType:
			// If parent block is a RootBlock, then make sure that the block contains a transaction;
			// otherwise, the reference is invalid.
			if parentBlock.IsRootBlock() {
				parentModelBlock, exists := b.loadBlockFromStorage(parent.ID)
				if !exists {
					return nil, ierrors.Wrapf(err, "shallow like parent %s does not exist in storage", parent.ID.String())
				}

				if _, hasTx := parentModelBlock.SignedTransaction(); !hasTx {
					return nil, ierrors.Wrapf(err, "shallow like parent %s does not contain a conflicting transaction", parent.ID.String())
				}

				break
			}

			// Check whether the parent contains a conflicting TX,
			// otherwise reference is invalid and the block should be marked as invalid as well.
			if signedTransaction, hasTx := parentBlock.SignedTransaction(); !hasTx || !parentBlock.PayloadSpenderIDs().Has(signedTransaction.Transaction.MustID()) {
				return nil, ierrors.Wrapf(err, "shallow like parent %s does not contain a conflicting transaction", parent.ID.String())
			}

			spenderIDsToInherit.AddAll(parentBlock.PayloadSpenderIDs())
			//  remove all conflicting spenders from spenderIDsToInherit
			for _, spenderID := range parentBlock.PayloadSpenderIDs().ToSlice() {
				if conflictingSpends, exists := b.spendDAG.ConflictingSpenders(spenderID); exists {
					spenderIDsToInherit.DeleteAll(b.spendDAG.FutureCone(conflictingSpends))
				}
			}
		}
	}

	// Add all spenders from the block's payload itself.
	// Forking on booking: we determine the block's PayloadSpenderIDs by treating each TX as a spender.
	spenderIDsToInherit.AddAll(block.PayloadSpenderIDs())

	// Only inherit spenders that are not yet accepted (aka merge to master).
	return b.spendDAG.UnacceptedSpenders(spenderIDsToInherit), nil
}
