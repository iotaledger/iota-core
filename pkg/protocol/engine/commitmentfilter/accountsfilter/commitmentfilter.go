package accountsfilter

import (
	"fmt"
	"sync"

	"github.com/iotaledger/hive.go/core/memstorage"
	hiveEd25519 "github.com/iotaledger/hive.go/crypto/ed25519"
	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/commitmentfilter"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/notarization"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/pkg/errors"
)

var (
	ErrInvalidSignature = errors.New("invalid signature")
)

type CommitmentFilter struct {
	// Events contains the Events of the CommitmentFilter
	events *commitmentfilter.Events
	// futureBlocks contains blocks with a commitment in the future, that should not be passed to the blockdag yet.
	futureBlocks *memstorage.IndexedStorage[iotago.SlotIndex, iotago.CommitmentID, *advancedset.AdvancedSet[*model.Block]]

	// commitmentFunc is a function that returns the commitment corresponding to the given slot index.
	commitmentFunc func(iotago.SlotIndex) (*model.Commitment, error)

	accountRetrieveFunc func(accountID iotago.AccountID, targetIndex iotago.SlotIndex) (accountData *accounts.AccountData, exists bool, err error)

	futureBlocksMutex sync.RWMutex

	nextIndexToPromote iotago.SlotIndex

	module.Module
}

func NewProvider(opts ...options.Option[CommitmentFilter]) module.Provider[*engine.Engine, commitmentfilter.CommitmentFilter] {
	return module.Provide(func(e *engine.Engine) commitmentfilter.CommitmentFilter {
		// TODO: check the accounts manager directly rather than loading the commitment from storage.
		c := New(e.Storage.Commitments().Load, e.Ledger.Account, opts...)
		e.HookConstructed(func() {
			e.Events.Notarization.SlotCommitted.Hook(func(details *notarization.SlotCommittedDetails) {
				c.PromoteFutureBlocksUntil(details.Commitment.Index())
			})

			e.Events.Filter.BlockPreAllowed.Hook(func(block *model.Block) {
				c.ProcessPreFilteredBlock(block)
			})

			e.Events.CommitmentFilter.LinkTo(c.events)

			c.TriggerInitialized()
		})

		return c
	})
}

func New(commitmentFunc func(iotago.SlotIndex) (*model.Commitment, error), accountRetrieveFunc func(accountID iotago.AccountID, targetIndex iotago.SlotIndex) (accountData *accounts.AccountData, exists bool, err error), opts ...options.Option[CommitmentFilter]) *CommitmentFilter {
	return options.Apply(&CommitmentFilter{
		futureBlocks:        memstorage.NewIndexedStorage[iotago.SlotIndex, iotago.CommitmentID, *advancedset.AdvancedSet[*model.Block]](),
		commitmentFunc:      commitmentFunc,
		accountRetrieveFunc: accountRetrieveFunc,
	}, opts,
	)
}

func (c *CommitmentFilter) PromoteFutureBlocksUntil(index iotago.SlotIndex) {
	c.futureBlocksMutex.Lock()
	defer c.futureBlocksMutex.Unlock()

	for i := c.nextIndexToPromote; i <= index; i++ {
		cm, err := c.commitmentFunc(i)
		if err != nil {
			panic(fmt.Sprintf("failed to load commitment for index %d: %s", i, err))
		}
		if storage := c.futureBlocks.Get(i, false); storage != nil {
			if futureBlocks, exists := storage.Get(cm.ID()); exists {
				_ = futureBlocks.ForEach(func(futureBlock *model.Block) (err error) {
					c.ProcessPreFilteredBlock(futureBlock)
					return nil
				})
			}
		}
		c.futureBlocks.Evict(i)
	}

	c.nextIndexToPromote = index + 1
}

func (c *CommitmentFilter) isFutureBlock(block *model.Block) (isFutureBlock bool) {
	c.futureBlocksMutex.RLock()
	defer c.futureBlocksMutex.RUnlock()

	// If we are not able to load the commitment for the block, it means we haven't committed this slot yet.
	if _, err := c.commitmentFunc(block.SlotCommitment().Index()); err != nil {
		lo.Return1(c.futureBlocks.Get(block.SlotCommitment().Index(), true).GetOrCreate(block.Block().SlotCommitment.MustID(), func() *advancedset.AdvancedSet[*model.Block] {
			return advancedset.New[*model.Block]()
		})).Add(block)

		return true
	}

	return false
}

func (c *CommitmentFilter) ProcessPreFilteredBlock(block *model.Block) {
	if c.isFutureBlock(block) {
		return
	}

	// check if the account exists in the specified slot.
	accountData, exists, err := c.accountRetrieveFunc(block.Block().IssuerID, block.SlotCommitment().Index())
	if err != nil {
		c.events.BlockFiltered.Trigger(&commitmentfilter.BlockFilteredEvent{
			Block:  block,
			Reason: errors.Wrapf(err, "could not retrieve account information for block issuer %s", block.Block().IssuerID),
		})

		return
	}
	if !exists {
		c.events.BlockFiltered.Trigger(&commitmentfilter.BlockFilteredEvent{
			Block:  block,
			Reason: errors.Wrapf(err, "block issuer account %s does not exist in slot commitment %s", block.Block().IssuerID, block.SlotCommitment().Index()),
		})

		return
	}

	// Check that the issuer key is valid for this block issuer and that the signature is valid
	edSig, isEdSig := block.Block().Signature.(*iotago.Ed25519Signature)
	if !isEdSig {
		c.events.BlockFiltered.Trigger(&commitmentfilter.BlockFilteredEvent{
			Block:  block,
			Reason: errors.WithMessagef(ErrInvalidSignature, "only ed2519 signatures supported, got %s", block.Block().Signature.Type()),
		})

		return
	}
	if !accountData.PubKeys.Has(edSig.PublicKey) {
		c.events.BlockFiltered.Trigger(&commitmentfilter.BlockFilteredEvent{
			Block:  block,
			Reason: errors.WithMessagef(ErrInvalidSignature, "block issuer account %s does not have public key %s in slot %d", block.Block().IssuerID, edSig.PublicKey, block.SlotCommitment().Index()),
		})

		return
	}
	signingMessage, err := block.Block().SigningMessage()
	hiveEd25519.Verify(edSig.PublicKey[:], signingMessage, edSig.Signature[:])
	if err != nil {
		c.events.BlockFiltered.Trigger(&commitmentfilter.BlockFilteredEvent{
			Block:  block,
			Reason: errors.WithMessagef(ErrInvalidSignature, "error: %s", err.Error()),
		})

		return
	}
	if !hiveEd25519.Verify(edSig.PublicKey[:], signingMessage, edSig.Signature[:]) {
		c.events.BlockFiltered.Trigger(&commitmentfilter.BlockFilteredEvent{
			Block:  block,
			Reason: ErrInvalidSignature,
		})

		return
	}

	// Check that the issuer of this block has non-negative block issuance credit
	if accountData.Credits.Value < 0 {
		c.events.BlockFiltered.Trigger(&commitmentfilter.BlockFilteredEvent{
			Block:  block,
			Reason: errors.Wrapf(err, "block issuer account %s is locked due to negative BIC", block.Block().IssuerID),
		})

		return
	}

	c.events.BlockAllowed.Trigger(block)
}

func (c *CommitmentFilter) Shutdown() {
	c.TriggerStopped()
}
