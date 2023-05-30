package prunable

import (
	"github.com/iotaledger/hive.go/core/storable"
	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/kvstore"
	iotago "github.com/iotaledger/iota.go/v4"
)

type BicDiffs struct {
	slot  iotago.SlotIndex
	store *kvstore.TypedStore[iotago.AccountID, storable.SerializableInt64, *iotago.AccountID, *storable.SerializableInt64]
}

// NewBicDiffs creates a new BicDiffs instance.
func NewBicDiffs(slot iotago.SlotIndex, store kvstore.KVStore) *BicDiffs {
	return &BicDiffs{
		slot:  slot,
		store: kvstore.NewTypedStore[iotago.AccountID, storable.SerializableInt64](store),
	}
}

// Store stores the given accountID as a root block.
func (r *BicDiffs) Store(accountID iotago.AccountID, change int64) (err error) {
	return r.store.Set(accountID, storable.SerializableInt64(change))
}

// Load loads accountID and commitmentID for the given blockID.
func (r *BicDiffs) Load(accountID iotago.AccountID) (int64, error) {
	storableInt64, err := r.store.Get(accountID)
	if err != nil {
		return 0, errors.Wrapf(err, "failed to get BIC diff for account %s", accountID.String())
	}
	return int64(storableInt64), nil
}

// Has returns true if the given accountID is a root block.
func (r *BicDiffs) Has(accountID iotago.AccountID) (has bool, err error) {
	return r.store.Has(accountID)
}

// Delete deletes the given accountID from the root blocks.
func (r *BicDiffs) Delete(accountID iotago.AccountID) (err error) {
	return r.store.Delete(accountID)
}

// Stream streams all accountIDs for a slot index.
func (r *BicDiffs) Stream(consumer func(accountID iotago.AccountID, change int64) bool) error {
	if storageErr := r.store.Iterate(kvstore.EmptyPrefix, func(accountID iotago.AccountID, storableChange storable.SerializableInt64) (advance bool) {
		return consumer(accountID, int64(storableChange))
	}); storageErr != nil {
		return errors.Wrapf(storageErr, "failed to iterate over bic diffs for slot %s", r.slot)
	}

	return nil
}
