package accountsfilter

import (
	"github.com/iotaledger/hive.go/core/memstorage"
	hiveEd25519 "github.com/iotaledger/hive.go/crypto/ed25519"
	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/commitmentfilter"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

var (
	ErrInvalidSignature = ierrors.New("invalid signature")
	ErrNegativeBIC      = ierrors.New("negative BIC")
	ErrAccountExpired   = ierrors.New("account expired")
)

type CommitmentFilter struct {
	// Events contains the Events of the CommitmentFilter
	events *commitmentfilter.Events
	// futureBlocks contains blocks with a commitment in the future, that should not be passed to the blockdag yet.
	futureBlocks *memstorage.IndexedStorage[iotago.SlotIndex, iotago.CommitmentID, ds.Set[*model.Block]]

	apiProvider api.Provider

	// commitmentFunc is a function that returns the commitment corresponding to the given slot index.
	commitmentFunc func(iotago.SlotIndex) (*model.Commitment, error)

	accountRetrieveFunc func(accountID iotago.AccountID, targetIndex iotago.SlotIndex) (*accounts.AccountData, bool, error)

	module.Module
}

func NewProvider(opts ...options.Option[CommitmentFilter]) module.Provider[*engine.Engine, commitmentfilter.CommitmentFilter] {
	return module.Provide(func(e *engine.Engine) commitmentfilter.CommitmentFilter {
		// TODO: check the accounts manager directly rather than loading the commitment from storage.
		c := New(e, opts...)
		e.HookConstructed(func() {
			c.commitmentFunc = e.Storage.Commitments().Load

			c.accountRetrieveFunc = e.Ledger.Account

			e.Events.Filter.BlockPreAllowed.Hook(c.ProcessPreFilteredBlock)
			e.Events.CommitmentFilter.LinkTo(c.events)

			c.TriggerInitialized()
		})

		return c
	})
}

func New(apiProvider api.Provider, opts ...options.Option[CommitmentFilter]) *CommitmentFilter {
	return options.Apply(&CommitmentFilter{
		apiProvider:  apiProvider,
		events:       commitmentfilter.NewEvents(),
		futureBlocks: memstorage.NewIndexedStorage[iotago.SlotIndex, iotago.CommitmentID, ds.Set[*model.Block]](),
	}, opts,
	)
}

func (c *CommitmentFilter) ProcessPreFilteredBlock(block *model.Block) {
	if c.isFutureBlock(block) {
		return
	}

	c.evaluateBlock(block)
}

func (c *CommitmentFilter) evaluateBlock(block *model.Block) {
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
			Reason: ierrors.Errorf("block issuer account %s does not exist in slot commitment %s", block.ProtocolBlock().IssuerID, block.ProtocolBlock().SlotCommitmentID.Index()),
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

	// Check that the account is not expired
	if accountData.ExpirySlot < block.ProtocolBlock().SlotCommitmentID.Index() {
		c.events.BlockFiltered.Trigger(&commitmentfilter.BlockFilteredEvent{
			Block:  block,
			Reason: ierrors.Wrapf(ErrAccountExpired, "block issuer account %s is expired, expiry slot %d in commitment %d", block.ProtocolBlock().IssuerID, accountData.ExpirySlot, block.ProtocolBlock().SlotCommitmentID.Index()),
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

	c.events.BlockAllowed.Trigger(block)
}

func (c *CommitmentFilter) Shutdown() {
	c.TriggerStopped()
}
