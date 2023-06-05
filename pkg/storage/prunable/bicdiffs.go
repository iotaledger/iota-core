package prunable

import (
	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/crypto/ed25519"
	"github.com/iotaledger/hive.go/ds/types"

	"github.com/iotaledger/hive.go/kvstore"
	iotago "github.com/iotaledger/iota.go/v4"
)

// BICDiff represent the storable changes for a single account within an slot.
type BICDiff struct {
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

// NewBICDiff creates a new BICDiff instance.
func NewBICDiff(api iotago.API) *BICDiff {
	return &BICDiff{
		api:                 api,
		Change:              0,
		PreviousUpdatedTime: 0,
		NewOutputID:         iotago.OutputID{},
		PreviousOutputID:    iotago.OutputID{},
		PubKeysAdded:        make([]ed25519.PublicKey, 0),
		PubKeysRemoved:      make([]ed25519.PublicKey, 0),
	}
}

func (b BICDiff) Bytes() ([]byte, error) {
	return b.api.Encode(b)
}

func (b *BICDiff) FromBytes(bytes []byte) (int, error) {
	return b.api.Decode(bytes, b)
}

// BICDiffs is the storable unit of BIC changes for all account in a slot.
type BICDiffs struct {
	api               iotago.API
	slot              iotago.SlotIndex
	diffChangeStore   *kvstore.TypedStore[iotago.AccountID, BICDiff, *iotago.AccountID, *BICDiff]
	destroyedAccounts *kvstore.TypedStore[iotago.AccountID, types.Empty, *iotago.AccountID, *types.Empty] // TODO is there any store for set of keys only?
}

// NewBICDiffs creates a new BICDiffs instance.
func NewBICDiffs(slot iotago.SlotIndex, store kvstore.KVStore, api iotago.API) *BICDiffs {
	return &BICDiffs{
		api:               api,
		slot:              slot,
		diffChangeStore:   kvstore.NewTypedStore[iotago.AccountID, BICDiff](store),
		destroyedAccounts: kvstore.NewTypedStore[iotago.AccountID, types.Empty](store),
	}
}

// Store stores the given accountID as a root block.
func (b *BICDiffs) Store(accountID iotago.AccountID, bicDiffChange BICDiff, destroyed bool) (err error) {
	if destroyed {
		if err := b.destroyedAccounts.Set(accountID, types.Void); err != nil {
			return errors.Wrapf(err, "failed to set destroyed account")
		}
	}

	return b.diffChangeStore.Set(accountID, bicDiffChange)
}

// Load loads accountID and commitmentID for the given blockID.
func (b *BICDiffs) Load(accountID iotago.AccountID) (bicDiffChange BICDiff, destroyed bool, err error) {
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
func (b *BICDiffs) Has(accountID iotago.AccountID) (has bool, err error) {
	return b.diffChangeStore.Has(accountID)
}

// Delete deletes the given accountID from the root blocks.
func (b *BICDiffs) Delete(accountID iotago.AccountID) (err error) {
	return b.diffChangeStore.Delete(accountID)
}

// Stream streams all accountIDs changes for a slot index.
func (b *BICDiffs) Stream(consumer func(accountID iotago.AccountID, bicDiffChange BICDiff, destroyed bool) bool) error {
	// We firstly iterate over the destroyed accounts, as they won't have a corresponding bicDiffChange.
	if storageErr := b.destroyedAccounts.Iterate(kvstore.EmptyPrefix, func(accountID iotago.AccountID, empty types.Empty) bool {
		return consumer(accountID, BICDiff{}, true)
	}); storageErr != nil {
		return errors.Wrapf(storageErr, "failed to iterate over bic diffs for slot %s", b.slot)
	}

	// For those accounts that still exist we might have a bicDiffChange.
	if storageErr := b.diffChangeStore.Iterate(kvstore.EmptyPrefix, func(accountID iotago.AccountID, bicDiffChange BICDiff) bool {
		return consumer(accountID, bicDiffChange, false)
	}); storageErr != nil {
		return errors.Wrapf(storageErr, "failed to iterate over bic diffs for slot %s", b.slot)
	}

	return nil
}

// StreamDestroyed streams all destroyed accountIDs for a slot index.
func (b *BICDiffs) StreamDestroyed(consumer func(accountID iotago.AccountID) bool) error {
	return b.destroyedAccounts.Iterate(kvstore.EmptyPrefix, func(accountID iotago.AccountID, empty types.Empty) bool {
		return consumer(accountID)
	})
}
