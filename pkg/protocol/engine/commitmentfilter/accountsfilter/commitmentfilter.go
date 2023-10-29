package accountsfilter

import (
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
		e.Constructed.OnTrigger(func() {
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
	accountData, exists, err := c.accountRetrieveFunc(block.ProtocolBlock().IssuerID, block.SlotCommitmentID().Slot())
	if err != nil {
		c.events.BlockFiltered.Trigger(&commitmentfilter.BlockFilteredEvent{
			Block:  block,
			Reason: ierrors.Join(iotago.ErrIssuerAccountNotFound, ierrors.Wrapf(err, "could not retrieve account information for block issuer %s", block.ProtocolBlock().IssuerID)),
		})

		return
	}
	if !exists {
		c.events.BlockFiltered.Trigger(&commitmentfilter.BlockFilteredEvent{
			Block:  block,
			Reason: ierrors.Join(iotago.ErrIssuerAccountNotFound, ierrors.Errorf("block issuer account %s does not exist in slot commitment %s", block.ProtocolBlock().IssuerID, block.ProtocolBlock().SlotCommitmentID.Slot())),
		})

		return
	}

	// get the api for the block
	blockAPI, err := c.apiProvider.APIForVersion(block.ProtocolBlock().BlockHeader.ProtocolVersion)
	if err != nil {
		c.events.BlockFiltered.Trigger(&commitmentfilter.BlockFilteredEvent{
			Block:  block,
			Reason: ierrors.Join(iotago.ErrBlockVersionInvalid, ierrors.Wrapf(err, "could not retrieve API for block version %d", block.ProtocolBlock().BlockHeader.ProtocolVersion)),
		})
	}
	// check that the block burns sufficient Mana, use slot index of the commitment
	rmcSlot := block.ProtocolBlock().SlotCommitmentID.Slot()

	rmc, err := c.rmcRetrieveFunc(rmcSlot)
	if err != nil {
		c.events.BlockFiltered.Trigger(&commitmentfilter.BlockFilteredEvent{
			Block:  block,
			Reason: ierrors.Join(iotago.ErrRMCNotFound, ierrors.Wrapf(err, "could not retrieve RMC for slot commitment %s", rmcSlot)),
		})

		return
	}
	if basicBlock, isBasic := block.BasicBlock(); isBasic {
		manaCost, err := basicBlock.ManaCost(rmc, blockAPI.ProtocolParameters().WorkScoreParameters())
		if err != nil {
			c.events.BlockFiltered.Trigger(&commitmentfilter.BlockFilteredEvent{
				Block:  block,
				Reason: ierrors.Join(iotago.ErrFailedToCalculateManaCost, ierrors.Wrapf(err, "could not calculate Mana cost for block")),
			})
		}
		if basicBlock.MaxBurnedMana < manaCost {
			c.events.BlockFiltered.Trigger(&commitmentfilter.BlockFilteredEvent{
				Block:  block,
				Reason: ierrors.Join(iotago.ErrBurnedInsufficientMana, ierrors.Errorf("block issuer account %s burned insufficient Mana, required %d, burned %d", block.ProtocolBlock().IssuerID, manaCost, basicBlock.MaxBurnedMana)),
			})

			return
		}
	}

	// Check that the issuer of this block has non-negative block issuance credit
	if accountData.Credits.Value < 0 {
		c.events.BlockFiltered.Trigger(&commitmentfilter.BlockFilteredEvent{
			Block:  block,
			Reason: ierrors.Wrapf(iotago.ErrNegativeBIC, "block issuer account %s is locked due to negative BIC", block.ProtocolBlock().IssuerID),
		})

		return
	}

	// Check that the account is not expired
	if accountData.ExpirySlot < block.ProtocolBlock().SlotCommitmentID.Slot() {
		c.events.BlockFiltered.Trigger(&commitmentfilter.BlockFilteredEvent{
			Block:  block,
			Reason: ierrors.Wrapf(iotago.ErrAccountExpired, "block issuer account %s is expired, expiry slot %d in commitment %d", block.ProtocolBlock().IssuerID, accountData.ExpirySlot, block.ProtocolBlock().SlotCommitmentID.Slot()),
		})

		return
	}

	switch signature := block.ProtocolBlock().Signature.(type) {
	case *iotago.Ed25519Signature:
		if !accountData.BlockIssuerKeys.Has(iotago.Ed25519PublicKeyBlockIssuerKeyFromPublicKey(signature.PublicKey)) {
			// If the block issuer does not have the public key in the slot commitment, check if it is an implicit account with the corresponding address.
			// There must be at least one block issuer key on any account, so extracting index 0 is fine.
			// For implicit accounts there is exactly one key, so we do not have to check any other indices.
			blockIssuerKey := accountData.BlockIssuerKeys[0]
			// Implicit Accounts can only have Block Issuer Keys of type Ed25519PublicKeyHashBlockIssuerKey.
			bikPubKeyHash, isBikPubKeyHash := blockIssuerKey.(*iotago.Ed25519PublicKeyHashBlockIssuerKey)

			// Filter the block if it's not a Block Issuer Key from an Implicit Account or if the Pub Key Hashes do not match.
			if !isBikPubKeyHash || bikPubKeyHash.PublicKeyHash != iotago.Ed25519PublicKeyHashBlockIssuerKeyFromPublicKey(signature.PublicKey[:]).PublicKeyHash {
				c.events.BlockFiltered.Trigger(&commitmentfilter.BlockFilteredEvent{
					Block:  block,
					Reason: ierrors.Wrapf(iotago.ErrInvalidSignature, "block issuer account %s does not have block issuer key corresponding to public key %s in slot %d", block.ProtocolBlock().IssuerID, signature.PublicKey, block.ProtocolBlock().SlotCommitmentID.Index()),
				})

				return
			}
		}
		signingMessage, err := block.ProtocolBlock().SigningMessage()
		if err != nil {
			c.events.BlockFiltered.Trigger(&commitmentfilter.BlockFilteredEvent{
				Block:  block,
				Reason: ierrors.Wrapf(iotago.ErrInvalidSignature, "error: %s", err.Error()),
			})

			return
		}
		if !hiveEd25519.Verify(signature.PublicKey[:], signingMessage, signature.Signature[:]) {
			c.events.BlockFiltered.Trigger(&commitmentfilter.BlockFilteredEvent{
				Block:  block,
				Reason: iotago.ErrInvalidSignature,
			})

			return
		}
	default:
		c.events.BlockFiltered.Trigger(&commitmentfilter.BlockFilteredEvent{
			Block:  block,
			Reason: ierrors.Wrapf(iotago.ErrInvalidSignature, "only ed25519 signatures supported, got %s", block.ProtocolBlock().Signature.Type()),
		})

		return
	}

	c.events.BlockAllowed.Trigger(block)
}

// Reset resets the component to a clean state as if it was created at the last commitment.
func (c *CommitmentFilter) Reset() {}

func (c *CommitmentFilter) Shutdown() {
	c.TriggerStopped()
}
