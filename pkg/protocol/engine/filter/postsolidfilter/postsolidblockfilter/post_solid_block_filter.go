package postsolidblockfilter

import (
	hiveEd25519 "github.com/iotaledger/hive.go/crypto/ed25519"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/filter/postsolidfilter"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/hexutil"
)

type PostSolidBlockFilter struct {
	// Events contains the Events of the PostSolidBlockFilter
	events *postsolidfilter.Events

	rmcRetrieveFunc func(iotago.SlotIndex) (iotago.Mana, error)

	accountRetrieveFunc func(accountID iotago.AccountID, targetIndex iotago.SlotIndex) (*accounts.AccountData, bool, error)

	blockCacheRetrieveFunc func(iotago.BlockID) (*blocks.Block, bool)

	module.Module
}

func NewProvider(opts ...options.Option[PostSolidBlockFilter]) module.Provider[*engine.Engine, postsolidfilter.PostSolidFilter] {
	return module.Provide(func(e *engine.Engine) postsolidfilter.PostSolidFilter {
		c := New(opts...)
		e.Constructed.OnTrigger(func() {
			c.accountRetrieveFunc = e.Ledger.Account
			c.blockCacheRetrieveFunc = e.BlockCache.Block

			e.Ledger.HookConstructed(func() {
				c.rmcRetrieveFunc = e.Ledger.RMCManager().RMC
			})

			e.Events.BlockDAG.BlockSolid.Hook(c.ProcessSolidBlock)
			e.Events.PostSolidFilter.LinkTo(c.events)

			c.TriggerInitialized()
		})

		return c
	})
}

func New(opts ...options.Option[PostSolidBlockFilter]) *PostSolidBlockFilter {
	return options.Apply(&PostSolidBlockFilter{
		events: postsolidfilter.NewEvents(),
	}, opts,
	)
}

func (c *PostSolidBlockFilter) ProcessSolidBlock(block *blocks.Block) {
	// Block issuing time monotonicity: a block's issuing time needs to be greater than its parents issuing time.
	{
		for _, parentID := range block.Parents() {
			parent, exists := c.blockCacheRetrieveFunc(parentID)
			if !exists {
				c.filterBlock(
					block,
					ierrors.Join(iotago.ErrBlockParentNotFound, ierrors.Errorf("parent %s of block %s is not known", parentID, block.ID())),
				)

				return
			}

			if !block.IssuingTime().After(parent.IssuingTime()) {
				c.filterBlock(
					block,
					ierrors.Join(iotago.ErrBlockIssuingTimeNonMonotonic, ierrors.Errorf("block %s issuing time %s not greater than parent's %s issuing time %s", block.ID(), block.IssuingTime(), parentID, parent.IssuingTime())),
				)

				return
			}
		}
	}

	// Perform account related checks.
	{
		// check if the account exists in the specified slot.
		accountData, exists, err := c.accountRetrieveFunc(block.ProtocolBlock().Header.IssuerID, block.SlotCommitmentID().Slot())
		if err != nil {
			c.filterBlock(
				block,
				ierrors.Join(iotago.ErrIssuerAccountNotFound, ierrors.Wrapf(err, "could not retrieve account information for block issuer %s", block.ProtocolBlock().Header.IssuerID)),
			)

			return
		}
		if !exists {
			c.filterBlock(
				block,
				ierrors.Join(iotago.ErrIssuerAccountNotFound, ierrors.Errorf("block issuer account %s does not exist in slot commitment %s", block.ProtocolBlock().Header.IssuerID, block.ProtocolBlock().Header.SlotCommitmentID.Slot())),
			)

			return
		}

		// check that the block burns sufficient Mana, use slot index of the commitment
		{
			rmcSlot := block.ProtocolBlock().Header.SlotCommitmentID.Slot()
			rmc, err := c.rmcRetrieveFunc(rmcSlot)
			if err != nil {
				c.filterBlock(
					block,
					ierrors.Join(iotago.ErrRMCNotFound, ierrors.Wrapf(err, "could not retrieve RMC for slot commitment %s", rmcSlot)),
				)

				return
			}
			if basicBlock, isBasic := block.BasicBlock(); isBasic {
				manaCost, err := block.ProtocolBlock().ManaCost(rmc)
				if err != nil {
					c.filterBlock(
						block,
						ierrors.Join(iotago.ErrFailedToCalculateManaCost, ierrors.Wrapf(err, "could not calculate Mana cost for block")),
					)
				}
				if basicBlock.MaxBurnedMana < manaCost {
					c.filterBlock(
						block,
						ierrors.Join(iotago.ErrBurnedInsufficientMana, ierrors.Errorf("block issuer account %s burned insufficient Mana, required %d, burned %d", block.ProtocolBlock().Header.IssuerID, manaCost, basicBlock.MaxBurnedMana)),
					)

					return
				}
			}
		}

		// Check that the issuer of this block has non-negative block issuance credit
		{
			if accountData.Credits.Value < 0 {
				c.filterBlock(
					block,
					ierrors.Wrapf(iotago.ErrAccountLocked, "block issuer account %s", block.ProtocolBlock().Header.IssuerID),
				)

				return
			}
		}

		// Check that the account is not expired
		{
			if accountData.ExpirySlot < block.ProtocolBlock().Header.SlotCommitmentID.Slot() {
				c.filterBlock(
					block,
					ierrors.Wrapf(iotago.ErrAccountExpired, "block issuer account %s is expired, expiry slot %d in commitment %d", block.ProtocolBlock().Header.IssuerID, accountData.ExpirySlot, block.ProtocolBlock().Header.SlotCommitmentID.Slot()),
				)

				return
			}
		}

		// Validate the signature of the block.
		{
			switch signature := block.ProtocolBlock().Signature.(type) {
			case *iotago.Ed25519Signature:
				expectedBlockIssuerKey := iotago.Ed25519PublicKeyHashBlockIssuerKeyFromPublicKey(signature.PublicKey)

				if !accountData.BlockIssuerKeys.Has(expectedBlockIssuerKey) {
					c.filterBlock(
						block,
						ierrors.Wrapf(iotago.ErrInvalidSignature, "block issuer account %s does not have block issuer key corresponding to public key %s in slot %d", block.ProtocolBlock().Header.IssuerID, hexutil.EncodeHex(signature.PublicKey[:]), block.ProtocolBlock().Header.SlotCommitmentID.Index()),
					)

					return
				}

				signingMessage, err := block.ProtocolBlock().SigningMessage()
				if err != nil {
					c.filterBlock(
						block,
						ierrors.Wrapf(iotago.ErrInvalidSignature, "error: %s", err.Error()),
					)

					return
				}
				if !hiveEd25519.Verify(signature.PublicKey[:], signingMessage, signature.Signature[:]) {
					c.filterBlock(
						block,
						iotago.ErrInvalidSignature,
					)

					return
				}
			default:
				c.filterBlock(
					block,
					ierrors.Wrapf(iotago.ErrInvalidSignature, "only ed25519 signatures supported, got %s", block.ProtocolBlock().Signature.Type()),
				)

				return
			}
		}
	}

	c.events.BlockAllowed.Trigger(block)
}

// Reset resets the component to a clean state as if it was created at the last commitment.
func (c *PostSolidBlockFilter) Reset() { /* nothing to reset but comply with interface */ }

func (c *PostSolidBlockFilter) Shutdown() {
	c.TriggerStopped()
}

func (c *PostSolidBlockFilter) filterBlock(block *blocks.Block, reason error) {
	block.SetInvalid()

	c.events.BlockFiltered.Trigger(&postsolidfilter.BlockFilteredEvent{
		Block:  block,
		Reason: reason,
	})
}
