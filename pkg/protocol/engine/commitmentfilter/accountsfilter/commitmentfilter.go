package accountsfilter

import (
	"github.com/iotaledger/hive.go/core/safemath"
	hiveEd25519 "github.com/iotaledger/hive.go/crypto/ed25519"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/commitmentfilter"
	iotago "github.com/iotaledger/iota.go/v4"
)

var (
	ErrInvalidSignature = ierrors.New("invalid signature")
	ErrNegativeBIC      = ierrors.New("negative BIC")
	ErrAccountExpired   = ierrors.New("account expired")
)

type CommitmentFilter struct {
	// Events contains the Events of the CommitmentFilter
	events *commitmentfilter.Events

	apiProvider iotago.APIProvider

	// commitmentFunc is a function that returns the commitment corresponding to the given slot index.
	commitmentFunc func(iotago.SlotIndex) (*model.Commitment, error)

	rmcRetrieveFunc func(iotago.SlotIndex) (iotago.Mana, error)

	accountRetrieveFunc func(accountID iotago.AccountID, targetIndex iotago.SlotIndex) (*accounts.AccountData, bool, error)

	module.Module
}

func NewProvider(opts ...options.Option[CommitmentFilter]) module.Provider[*engine.Engine, commitmentfilter.CommitmentFilter] {
	return module.Provide(func(e *engine.Engine) commitmentfilter.CommitmentFilter {
		c := New(e, opts...)
		e.HookConstructed(func() {
			c.commitmentFunc = e.Storage.Commitments().Load

			c.accountRetrieveFunc = e.Ledger.Account

			e.Ledger.HookConstructed(func() {
				c.rmcRetrieveFunc = e.Ledger.RMCManager().RMC
			})

			e.Events.BlockDAG.BlockSolid.Hook(c.ProcessPreFilteredBlock)
			e.Events.CommitmentFilter.LinkTo(c.events)

			c.TriggerInitialized()
		})

		return c
	})
}

func New(apiProvider iotago.APIProvider, opts ...options.Option[CommitmentFilter]) *CommitmentFilter {
	return options.Apply(&CommitmentFilter{
		apiProvider: apiProvider,
		events:      commitmentfilter.NewEvents(),
	}, opts,
	)
}

func (c *CommitmentFilter) ProcessPreFilteredBlock(block *blocks.Block) {
	c.evaluateBlock(block)
}

func (c *CommitmentFilter) evaluateBlock(block *blocks.Block) {
	// check if the account exists in the specified slot.
	accountData, exists, err := c.accountRetrieveFunc(block.ProtocolBlock().IssuerID, block.SlotCommitmentID().Index())
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

	// get the api for the block
	blockAPI, err := c.apiProvider.APIForVersion(block.ProtocolBlock().BlockHeader.ProtocolVersion)
	if err != nil {
		c.events.BlockFiltered.Trigger(&commitmentfilter.BlockFilteredEvent{
			Block:  block,
			Reason: ierrors.Wrapf(err, "could not retrieve API for block version %d", block.ProtocolBlock().BlockHeader.ProtocolVersion),
		})
	}
	// check that the block burns sufficient Mana
	blockSlot := blockAPI.TimeProvider().SlotFromTime(block.ProtocolBlock().IssuingTime)
	rmcSlot, err := safemath.SafeSub(blockSlot, blockAPI.ProtocolParameters().MaxCommittableAge())
	if err != nil {
		rmcSlot = 0
	}
	rmc, err := c.rmcRetrieveFunc(rmcSlot)
	if err != nil {
		c.events.BlockFiltered.Trigger(&commitmentfilter.BlockFilteredEvent{
			Block:  block,
			Reason: ierrors.Wrapf(err, "could not retrieve RMC for slot commitment %s", rmcSlot),
		})

		return
	}
	if basicBlock, isBasic := block.BasicBlock(); isBasic {
		manaCost, err := basicBlock.ManaCost(rmc, blockAPI.ProtocolParameters().WorkScoreStructure())
		if err != nil {
			c.events.BlockFiltered.Trigger(&commitmentfilter.BlockFilteredEvent{
				Block:  block,
				Reason: ierrors.Wrapf(err, "could not calculate Mana cost for block"),
			})
		}
		if basicBlock.BurnedMana < manaCost {
			c.events.BlockFiltered.Trigger(&commitmentfilter.BlockFilteredEvent{
				Block:  block,
				Reason: ierrors.Errorf("block issuer account %s burned insufficient Mana, required %d, burned %d", block.ProtocolBlock().IssuerID, manaCost, basicBlock.BurnedMana),
			})

			return
		}
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

	switch signature := block.ProtocolBlock().Signature.(type) {
	case *iotago.Ed25519Signature:
		if !accountData.BlockIssuerKeys.Has(iotago.BlockIssuerKeyEd25519FromPublicKey(signature.PublicKey)) {
			c.events.BlockFiltered.Trigger(&commitmentfilter.BlockFilteredEvent{
				Block:  block,
				Reason: ierrors.Wrapf(ErrInvalidSignature, "block issuer account %s does not have public key %s in slot %d", block.ProtocolBlock().IssuerID, signature.PublicKey, block.ProtocolBlock().SlotCommitmentID.Index()),
			})

			return
		}
		signingMessage, err := block.ProtocolBlock().SigningMessage(blockAPI)
		if err != nil {
			c.events.BlockFiltered.Trigger(&commitmentfilter.BlockFilteredEvent{
				Block:  block,
				Reason: ierrors.Wrapf(ErrInvalidSignature, "error: %s", err.Error()),
			})

			return
		}
		if !hiveEd25519.Verify(signature.PublicKey[:], signingMessage, signature.Signature[:]) {
			c.events.BlockFiltered.Trigger(&commitmentfilter.BlockFilteredEvent{
				Block:  block,
				Reason: ErrInvalidSignature,
			})

			return
		}
	default:
		c.events.BlockFiltered.Trigger(&commitmentfilter.BlockFilteredEvent{
			Block:  block,
			Reason: ierrors.Wrapf(ErrInvalidSignature, "only ed25519 signatures supported, got %s", block.ProtocolBlock().Signature.Type()),
		})

		return
	}

	c.events.BlockAllowed.Trigger(block)
}

func (c *CommitmentFilter) Shutdown() {
	c.TriggerStopped()
}
