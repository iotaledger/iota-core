package prunable

import (
	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/crypto/ed25519"
	"github.com/iotaledger/hive.go/ds/types"

	"github.com/iotaledger/hive.go/kvstore"
	iotago "github.com/iotaledger/iota.go/v4"
)

// AccountDiff represent the storable changes for a single account within an slot.
type AccountDiff struct {
	api                 iotago.API
	Change              int64            `serix:"0"`
	PreviousUpdatedTime iotago.SlotIndex `serix:"1"`

	// OutputID to which the Account has been transitioned to.
	NewOutputID iotago.OutputID `serix:"2"`

	// OutputID from which the Account has been transitioned from.
	PreviousOutputID iotago.OutputID `serix:"3"`

	PubKeysAdded   []ed25519.PublicKey `serix:"4"`
	PubKeysRemoved []ed25519.PublicKey `serix:"5"`
}

// NewAccountDiff creates a new AccountDiff instance.
func NewAccountDiff(api iotago.API) *AccountDiff {
	return &AccountDiff{
		api:                 api,
		Change:              0,
		PreviousUpdatedTime: 0,
		NewOutputID:         iotago.OutputID{},
		PreviousOutputID:    iotago.OutputID{},
		PubKeysAdded:        make([]ed25519.PublicKey, 0),
		PubKeysRemoved:      make([]ed25519.PublicKey, 0),
	}
}

func (b AccountDiff) Bytes() ([]byte, error) {
	return b.api.Encode(b)
}

func (b *AccountDiff) FromBytes(bytes []byte) (int, error) {
	return b.api.Decode(bytes, b)
}

// AccountDiffs is the storable unit of Account changes for all account in a slot.
type AccountDiffs struct {
	api               iotago.API
	slot              iotago.SlotIndex
	diffChangeStore   *kvstore.TypedStore[iotago.AccountID, AccountDiff, *iotago.AccountID, *AccountDiff]
	destroyedAccounts *kvstore.TypedStore[iotago.AccountID, types.Empty, *iotago.AccountID, *types.Empty] // TODO is there any store for set of keys only?
}

// NewAccountDiffs creates a new AccountDiffs instance.
func NewAccountDiffs(slot iotago.SlotIndex, store kvstore.KVStore, api iotago.API) *AccountDiffs {
	return &AccountDiffs{
		api:               api,
		slot:              slot,
		diffChangeStore:   kvstore.NewTypedStore[iotago.AccountID, AccountDiff](store),
		destroyedAccounts: kvstore.NewTypedStore[iotago.AccountID, types.Empty](store),
	}
}

// Store stores the given accountID as a root block.
func (b *AccountDiffs) Store(accountID iotago.AccountID, accountDiff AccountDiff, destroyed bool) (err error) {
	if destroyed {
		if err := b.destroyedAccounts.Set(accountID, types.Void); err != nil {
			return errors.Wrapf(err, "failed to set destroyed account")
		}
	}

	return b.diffChangeStore.Set(accountID, accountDiff)
}

// Load loads accountID and commitmentID for the given blockID.
func (b *AccountDiffs) Load(accountID iotago.AccountID) (accountDiff AccountDiff, destroyed bool, err error) {
	if destroyed, err = b.destroyedAccounts.Has(accountID); err != nil {
		return accountDiff, false, errors.Wrapf(err, "failed to get destroyed account")
	} else if destroyed {
		return accountDiff, true, nil
	}

	accountDiff, err = b.diffChangeStore.Get(accountID)
	if err != nil {
		return accountDiff, false, errors.Wrapf(err, "failed to get Account diff for account %s", accountID.String())
	}

	return accountDiff, false, err
}

// Has returns true if the given accountID is a root block.
func (b *AccountDiffs) Has(accountID iotago.AccountID) (has bool, err error) {
	return b.diffChangeStore.Has(accountID)
}

// Delete deletes the given accountID from the root blocks.
func (b *AccountDiffs) Delete(accountID iotago.AccountID) (err error) {
	return b.diffChangeStore.Delete(accountID)
}

// Stream streams all accountIDs changes for a slot index.
func (b *AccountDiffs) Stream(consumer func(accountID iotago.AccountID, accountDiff AccountDiff, destroyed bool) bool) error {
	// We firstly iterate over the destroyed accounts, as they won't have a corresponding accountDiff.
	if storageErr := b.destroyedAccounts.Iterate(kvstore.EmptyPrefix, func(accountID iotago.AccountID, empty types.Empty) bool {
		return consumer(accountID, AccountDiff{}, true)
	}); storageErr != nil {
		return errors.Wrapf(storageErr, "failed to iterate over account diffs for slot %s", b.slot)
	}

	// For those accounts that still exist we might have a accountDiff.
	if storageErr := b.diffChangeStore.Iterate(kvstore.EmptyPrefix, func(accountID iotago.AccountID, accountDiff AccountDiff) bool {
		return consumer(accountID, accountDiff, false)
	}); storageErr != nil {
		return errors.Wrapf(storageErr, "failed to iterate over account diffs for slot %s", b.slot)
	}

	return nil
}

// StreamDestroyed streams all destroyed accountIDs for a slot index.
func (b *AccountDiffs) StreamDestroyed(consumer func(accountID iotago.AccountID) bool) error {
	return b.destroyedAccounts.Iterate(kvstore.EmptyPrefix, func(accountID iotago.AccountID, empty types.Empty) bool {
		return consumer(accountID)
	})
}
