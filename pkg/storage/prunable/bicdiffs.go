package prunable

import (
	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/crypto/ed25519"
	"github.com/iotaledger/hive.go/ds/types"

	"github.com/iotaledger/hive.go/kvstore"
	iotago "github.com/iotaledger/iota.go/v4"
)

// BicDiffChange represent the storable changes of an account's BIC between two slots.
type BicDiffChange struct {
	api                 iotago.API
	Change              int64               `serix:"0"`
	PreviousUpdatedTime iotago.SlotIndex    `serix:"1"`
	PubKeysAdded        []ed25519.PublicKey `serix:"2"`
	PubKeysRemoved      []ed25519.PublicKey `serix:"3"`
}

func (c BicDiffChange) Bytes() ([]byte, error) {
	return c.api.Encode(c)
}

func (c *BicDiffChange) FromBytes(bytes []byte) (int, error) {
	return c.api.Decode(bytes, c)
}

type BicDiffs struct {
	api               iotago.API
	slot              iotago.SlotIndex
	diffChangeStore   *kvstore.TypedStore[iotago.AccountID, BicDiffChange, *iotago.AccountID, *BicDiffChange]
	destroyedAccounts *kvstore.TypedStore[iotago.AccountID, types.Empty, *iotago.AccountID, *types.Empty] // TODO is there any store for set of keys only?
}

// NewBicDiffs creates a new BicDiffs instance.
func NewBicDiffs(slot iotago.SlotIndex, store kvstore.KVStore, api iotago.API) *BicDiffs {
	return &BicDiffs{
		api:               api,
		slot:              slot,
		diffChangeStore:   kvstore.NewTypedStore[iotago.AccountID, BicDiffChange](store),
		destroyedAccounts: kvstore.NewTypedStore[iotago.AccountID, types.Empty](store),
	}
}

// Store stores the given accountID as a root block.
func (b *BicDiffs) Store(accountID iotago.AccountID, bicDiffChange BicDiffChange, destroyed bool) (err error) {
	if destroyed {
		if err := b.destroyedAccounts.Set(accountID, types.Void); err != nil {
			return errors.Wrapf(err, "failed to set destroyed account")
		}
	}

	return b.diffChangeStore.Set(accountID, bicDiffChange)
}

// Load loads accountID and commitmentID for the given blockID.
func (b *BicDiffs) Load(accountID iotago.AccountID) (bicDiffChange BicDiffChange, destroyed bool, err error) {
	if destroyed, err = b.destroyedAccounts.Has(accountID); err != nil {
		return bicDiffChange, false, errors.Wrapf(err, "failed to get destroyed account")
	} else if destroyed {
		return bicDiffChange, true, nil
	}

	bicDiffChange, err = b.diffChangeStore.Get(accountID)
	if err != nil {
		return bicDiffChange, false, errors.Wrapf(err, "failed to get BIC diff for account %s", accountID.String())
	}

	return bicDiffChange, false, err
}

// Has returns true if the given accountID is a root block.
func (b *BicDiffs) Has(accountID iotago.AccountID) (has bool, err error) {
	return b.diffChangeStore.Has(accountID)
}

// Delete deletes the given accountID from the root blocks.
func (b *BicDiffs) Delete(accountID iotago.AccountID) (err error) {
	return b.diffChangeStore.Delete(accountID)
}

// Stream streams all accountIDs changes for a slot index.
func (b *BicDiffs) Stream(consumer func(accountID iotago.AccountID, bicDiffChange BicDiffChange, destroyed bool) bool) error {
	// We firstly iterate over the destroyed accounts, as they won't have a corresponding bicDiffChange.
	if storageErr := b.destroyedAccounts.Iterate(kvstore.EmptyPrefix, func(accountID iotago.AccountID, empty types.Empty) bool {
		return consumer(accountID, BicDiffChange{}, true)
	}); storageErr != nil {
		return errors.Wrapf(storageErr, "failed to iterate over bic diffs for slot %s", b.slot)
	}

	// For those accounts that still exist we might have a bicDiffChange.
	if storageErr := b.diffChangeStore.Iterate(kvstore.EmptyPrefix, func(accountID iotago.AccountID, bicDiffChange BicDiffChange) bool {
		return consumer(accountID, bicDiffChange, false)
	}); storageErr != nil {
		return errors.Wrapf(storageErr, "failed to iterate over bic diffs for slot %s", b.slot)
	}

	return nil
}

func (b *BicDiffs) StreamDestroyed(consumer func(accountID iotago.AccountID) bool) error {
	return b.destroyedAccounts.Iterate(kvstore.EmptyPrefix, func(accountID iotago.AccountID, empty types.Empty) bool {
		return consumer(accountID)
	})
}
