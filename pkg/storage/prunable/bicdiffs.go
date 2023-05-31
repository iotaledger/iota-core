package prunable

import (
	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/crypto/ed25519"
	"github.com/iotaledger/hive.go/ds/types"

	"github.com/iotaledger/hive.go/kvstore"
	iotago "github.com/iotaledger/iota.go/v4"
)

// bicDiffChange represent the storable changes of an account's BIC between two slots.
type bicDiffChange struct {
	api            iotago.API
	Change         int64               `serix:"2"`
	PubKeysAdded   []ed25519.PublicKey `serix:"0"`
	PubKeysRemoved []ed25519.PublicKey `serix:"1"`
}

func (c bicDiffChange) Bytes() ([]byte, error) {
	return c.api.Encode(c)
}

func (c *bicDiffChange) FromBytes(bytes []byte) (int, error) {
	return c.api.Decode(bytes, c)
}

type BicDiffs struct {
	api               iotago.API
	slot              iotago.SlotIndex
	diffChangeStore   *kvstore.TypedStore[iotago.AccountID, bicDiffChange, *iotago.AccountID, *bicDiffChange]
	destroyedAccounts *kvstore.TypedStore[iotago.AccountID, types.Empty, *iotago.AccountID, *types.Empty] // TODO is there any store for set of keys only?
}

// NewBicDiffs creates a new BicDiffs instance.
func NewBicDiffs(slot iotago.SlotIndex, store kvstore.KVStore, api iotago.API) *BicDiffs {
	return &BicDiffs{
		api:               api,
		slot:              slot,
		diffChangeStore:   kvstore.NewTypedStore[iotago.AccountID, bicDiffChange](store),
		destroyedAccounts: kvstore.NewTypedStore[iotago.AccountID, types.Empty](store),
	}
}

// Store stores the given accountID as a root block.
func (b *BicDiffs) Store(accountID iotago.AccountID, change int64, addedPubKeys, removedPubKeys []ed25519.PublicKey, destroyed bool) (err error) {
	if destroyed {
		if err := b.destroyedAccounts.Set(accountID, types.Void); err != nil {
			return errors.Wrapf(err, "failed to set destroyed account")
		}
		return nil
	}

	bicDiffChange := bicDiffChange{
		api:            b.api,
		Change:         change,
		PubKeysAdded:   addedPubKeys,
		PubKeysRemoved: removedPubKeys,
	}
	return b.diffChangeStore.Set(accountID, bicDiffChange)
}

// Load loads accountID and commitmentID for the given blockID.
func (b *BicDiffs) Load(accountID iotago.AccountID) (change int64, addedPubKeys, removedPubKeys []ed25519.PublicKey, destroyed bool, err error) {
	if destroyed, err = b.destroyedAccounts.Has(accountID); err != nil {
		return 0, nil, nil, false, errors.Wrapf(err, "failed to get destroyed account")
	} else if destroyed {
		return 0, nil, nil, true, nil
	}

	bicDiffChange, err := b.diffChangeStore.Get(accountID)
	if err != nil {
		return 0, nil, nil, false, errors.Wrapf(err, "failed to get BIC diff for account %s", accountID.String())
	}

	return bicDiffChange.Change, bicDiffChange.PubKeysAdded, bicDiffChange.PubKeysRemoved, false, nil
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
func (b *BicDiffs) Stream(consumer func(accountID iotago.AccountID, change int64, addedPubKeys, removedPubKeys []ed25519.PublicKey, destroyed bool) bool) error {
	// We firstly iterate over the destroyed accounts, as they won't have a corresponding bicDiffChange.
	if storageErr := b.destroyedAccounts.Iterate(kvstore.EmptyPrefix, func(accountID iotago.AccountID, empty types.Empty) bool {
		return consumer(accountID, 0, nil, nil, true)
	}); storageErr != nil {
		return errors.Wrapf(storageErr, "failed to iterate over bic diffs for slot %s", b.slot)
	}

	// For those accounts that still exist we might have a bicDiffChange.
	if storageErr := b.diffChangeStore.Iterate(kvstore.EmptyPrefix, func(accountID iotago.AccountID, bicDiffChange bicDiffChange) bool {
		return consumer(accountID, bicDiffChange.Change, bicDiffChange.PubKeysAdded, bicDiffChange.PubKeysRemoved, false)
	}); storageErr != nil {
		return errors.Wrapf(storageErr, "failed to iterate over bic diffs for slot %s", b.slot)
	}

	return nil
}
