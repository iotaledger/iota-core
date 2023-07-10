package accountsfilter

import (
	"fmt"
	"sync"

	"github.com/iotaledger/hive.go/core/memstorage"
	hiveEd25519 "github.com/iotaledger/hive.go/crypto/ed25519"
	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/core/api"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/commitmentfilter"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/notarization"
	iotago "github.com/iotaledger/iota.go/v4"
)

var (
	ErrInvalidSignature = ierrors.New("invalid signature")
	ErrNegativeBIC      = ierrors.New("negative BIC")
)

type CommitmentFilter struct {
	// Events contains the Events of the CommitmentFilter
	events *commitmentfilter.Events
	// futureBlocks contains blocks with a commitment in the future, that should not be passed to the blockdag yet.
	futureBlocks *memstorage.IndexedStorage[iotago.SlotIndex, iotago.CommitmentID, *advancedset.AdvancedSet[*model.Block]]

	apiProvider api.Provider

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
		c := New(e.Storage.Commitments().Load, e.Ledger.Account, e, opts...)
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

func New(commitmentFunc func(iotago.SlotIndex) (*model.Commitment, error), accountRetrieveFunc func(accountID iotago.AccountID, targetIndex iotago.SlotIndex) (accountData *accounts.AccountData, exists bool, err error), apiProvider api.Provider, opts ...options.Option[CommitmentFilter]) *CommitmentFilter {
	return options.Apply(&CommitmentFilter{
		apiProvider:         apiProvider,
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
	if _, err := c.commitmentFunc(block.ProtocolBlock().SlotCommitmentID.Index()); err != nil {
		lo.Return1(c.futureBlocks.Get(block.ProtocolBlock().SlotCommitmentID.Index(), true).GetOrCreate(block.ProtocolBlock().SlotCommitmentID, func() *advancedset.AdvancedSet[*model.Block] {
			return advancedset.New[*model.Block]()
		})).Add(block)

		return true
	}

	return false
}

func (c *CommitmentFilter) ProcessPreFilteredBlock(block *model.Block) {
	// TODO: redo optimisations to mark future block rather than checking each time.
	if c.isFutureBlock(block) {
		return
	}

	// check if the account exists in the specified slot.
	accountData, exists, err := c.accountRetrieveFunc(block.ProtocolBlock().IssuerID, block.ProtocolBlock().SlotCommitmentID.Index())
	if err != nil {
		c.events.BlockFiltered.Trigger(&commitmentfilter.BlockFilteredEvent{
			Block:  block,
			Reason: ierrors.Wrapf(err, "could not retrieve account information for block issuer %s", block.ProtocolBlock().IssuerID),
		})

		return
	}
	if !exists {
		c.events.BlockFiltered.Trigger(&commitmentfilter.BlockFilteredEvent{
			Block:  block,
			Reason: ierrors.Wrapf(err, "block issuer account %s does not exist in slot commitment %s", block.ProtocolBlock().IssuerID, block.ProtocolBlock().SlotCommitmentID.Index()),
		})

		return
	}

	// Check that the issuer key is valid for this block issuer and that the signature is valid
	edSig, isEdSig := block.ProtocolBlock().Signature.(*iotago.Ed25519Signature)
	if !isEdSig {
		c.events.BlockFiltered.Trigger(&commitmentfilter.BlockFilteredEvent{
			Block:  block,
			Reason: ierrors.Wrapf(ErrInvalidSignature, "only ed2519 signatures supported, got %s", block.ProtocolBlock().Signature.Type()),
		})

		return
	}
	if !accountData.PubKeys.Has(edSig.PublicKey) {
		c.events.BlockFiltered.Trigger(&commitmentfilter.BlockFilteredEvent{
			Block:  block,
			Reason: ierrors.Wrapf(ErrInvalidSignature, "block issuer account %s does not have public key %s in slot %d", block.ProtocolBlock().IssuerID, edSig.PublicKey, block.ProtocolBlock().SlotCommitmentID.Index()),
		})

		return
	}
	signingMessage, err := block.ProtocolBlock().SigningMessage(c.apiProvider.LatestAPI())
	hiveEd25519.Verify(edSig.PublicKey[:], signingMessage, edSig.Signature[:])
	if err != nil {
		c.events.BlockFiltered.Trigger(&commitmentfilter.BlockFilteredEvent{
			Block:  block,
			Reason: ierrors.Wrapf(ErrInvalidSignature, "error: %s", err.Error()),
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
			Reason: ierrors.Wrapf(ErrNegativeBIC, "block issuer account %s is locked due to negative BIC", block.ProtocolBlock().IssuerID),
		})

		return
	}

	c.events.BlockAllowed.Trigger(block)
}

func (c *CommitmentFilter) Shutdown() {
	c.TriggerStopped()
}
