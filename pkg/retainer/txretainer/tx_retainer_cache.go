package txretainer

import (
	"sync"

	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	iotago "github.com/iotaledger/iota.go/v4"
)

// transactionRetainerCache is a cache for transaction metadata changes that are not yet committed to the database.
type transactionRetainerCache struct {
	accessMutex sync.RWMutex

	txMetadataUpdateFunc txMetadataUpdateFunc

	uncommittedTxMetadataChangesByID *shrinkingmap.ShrinkingMap[iotago.TransactionID, *TransactionMetadata]
	uncommittedTxIDsBySlot           *shrinkingmap.ShrinkingMap[iotago.SlotIndex, map[iotago.TransactionID]struct{}]
}

// defaultTxMetadataUpdateFunc is the default update function for transaction metadata in the transaction retainer cache.
// HINT: do not modify the contents of the old transaction metadata in the update function, as it is used as a reference
// to the earliest attachment slot in the cache.
func defaultTxMetadataUpdateFunc(oldTxMeta *TransactionMetadata, newTxMeta *TransactionMetadata) (*TransactionMetadata, bool, error) {
	if oldTxMeta == nil {
		// no former transaction metadata exists, return the new one
		return newTxMeta, true, nil
	}

	if oldTxMeta.Equal(newTxMeta) {
		// the transaction metadata has not changed, return the old one
		return oldTxMeta, false, nil
	}

	if oldTxMeta.ValidSignature && !newTxMeta.ValidSignature {
		// do not update the entry in the cache if the signature was valid before and is invalid now.
		// this can for example happen if there is a new attachment with an invalid signature.
		return oldTxMeta, false, nil
	}

	if !oldTxMeta.ValidSignature && newTxMeta.ValidSignature {
		// if the signature was invalid before and is valid now, update the entry in the cache.
		// all former fields are overwritten by the new transaction metadata, including the earliest attachment slot.
		return newTxMeta, true, nil
	}

	// update the new earliest attachment slot if it is not set in the new transaction metadata, but was set in the old one or it was smaller in the old one.
	if newTxMeta.EarliestAttachmentSlot == 0 || (oldTxMeta.EarliestAttachmentSlot != 0 && oldTxMeta.EarliestAttachmentSlot < newTxMeta.EarliestAttachmentSlot) {
		newTxMeta.EarliestAttachmentSlot = oldTxMeta.EarliestAttachmentSlot
	}

	return newTxMeta, true, nil
}

// WithTxRetainerCacheTxMetadataUpdateFunc is an option for the transaction retainer cache that allows to set a custom update function for transaction metadata.
func WithTxRetainerCacheTxMetadataUpdateFunc(updateFunc txMetadataUpdateFunc) options.Option[transactionRetainerCache] {
	return func(c *transactionRetainerCache) {
		c.txMetadataUpdateFunc = updateFunc
	}
}

// NewTransactionRetainerCache creates a new transaction retainer cache.
//
//nolint:revive // only used in the tests
func NewTransactionRetainerCache(opts ...options.Option[transactionRetainerCache]) *transactionRetainerCache {
	return options.Apply(&transactionRetainerCache{
		uncommittedTxMetadataChangesByID: shrinkingmap.New[iotago.TransactionID, *TransactionMetadata](),
		uncommittedTxIDsBySlot:           shrinkingmap.New[iotago.SlotIndex, map[iotago.TransactionID]struct{}](),
	}, opts, func(c *transactionRetainerCache) {
		if c.txMetadataUpdateFunc == nil {
			c.txMetadataUpdateFunc = defaultTxMetadataUpdateFunc
		}
	})
}

// Reset resets the cache.
func (c *transactionRetainerCache) Reset() {
	c.accessMutex.Lock()
	defer c.accessMutex.Unlock()

	c.uncommittedTxMetadataChangesByID.Clear()
	c.uncommittedTxIDsBySlot.Clear()
}

// UpdateTxMetadata updates the metadata of a transaction or adds a new one if it didn't exist in the cache before.
func (c *transactionRetainerCache) UpdateTxMetadata(newTxMeta *TransactionMetadata) error {
	c.accessMutex.Lock()
	defer c.accessMutex.Unlock()

	txID := iotago.TransactionID(newTxMeta.TransactionID)

	var innerErr error

	// update the transaction metadata
	c.uncommittedTxMetadataChangesByID.Compute(txID, func(oldTxMeta *TransactionMetadata, exists bool) *TransactionMetadata {
		// update the transaction metadata using the update function
		updatedTxMeta, updated, err := c.txMetadataUpdateFunc(oldTxMeta, newTxMeta)
		if err != nil {
			innerErr = err
			return oldTxMeta
		}

		// if the transaction metadata has not changed, return
		if !updated {
			return updatedTxMeta
		}

		// we need to keep the list of earliest attachment slots in sync with the transaction metadata changes
		if exists {
			// update the earliest attachment slot of the transaction if it has changed
			if oldTxMeta.EarliestAttachmentSlot != updatedTxMeta.EarliestAttachmentSlot {
				c.updateEarliestAttachmentSlotWithoutLocking(txID, oldTxMeta.EarliestAttachmentSlot, updatedTxMeta.EarliestAttachmentSlot)
			}
		} else {
			// add the earliest attachment slot of the new transaction
			c.addEarliestAttachmentSlotWithoutLocking(txID, updatedTxMeta.EarliestAttachmentSlot)
		}

		return updatedTxMeta
	})

	return innerErr
}

func (c *transactionRetainerCache) addEarliestAttachmentSlotWithoutLocking(txID iotago.TransactionID, slot iotago.SlotIndex) {
	uncommittedSlot, _ := c.uncommittedTxIDsBySlot.GetOrCreate(slot, func() map[iotago.TransactionID]struct{} {
		return make(map[iotago.TransactionID]struct{})
	})

	// add the transaction to the slot
	uncommittedSlot[txID] = struct{}{}
}

// updateEarliestAttachmentSlotWithoutLocking updates the earliest attachment slot of a transaction without locking the cache.
// the caller needs to ensure that old entry exists in the uncommittedTxIDsBySlot map.
func (c *transactionRetainerCache) updateEarliestAttachmentSlotWithoutLocking(txID iotago.TransactionID, oldSlot iotago.SlotIndex, newSlot iotago.SlotIndex) {
	// remove the transaction from the old slot
	delete(lo.Return1(c.uncommittedTxIDsBySlot.Get(oldSlot)), txID)

	// add the transaction to the new slot
	c.addEarliestAttachmentSlotWithoutLocking(txID, newSlot)
}

// UpdateEarliestAttachmentSlot updates the earliest attachment slot of a transaction.
func (c *transactionRetainerCache) UpdateEarliestAttachmentSlot(txID iotago.TransactionID, earliestAttachmentSlot iotago.SlotIndex) {
	c.accessMutex.Lock()
	defer c.accessMutex.Unlock()

	txMeta, exists := c.uncommittedTxMetadataChangesByID.Get(txID)
	if !exists {
		// the transaction metadata does not exist, no need to update the earliest attachment slot
		return
	}

	// update the earliest attachment slot of the transaction
	c.updateEarliestAttachmentSlotWithoutLocking(txID, txMeta.EarliestAttachmentSlot, earliestAttachmentSlot)

	// update the earliest attachment slot in the transaction metadata
	txMeta.EarliestAttachmentSlot = earliestAttachmentSlot
}

// TransactionMetadataByID returns the transaction metadata of a transaction by its ID.
func (c *transactionRetainerCache) TransactionMetadataByID(txID iotago.TransactionID) (*TransactionMetadata, bool) {
	c.accessMutex.RLock()
	defer c.accessMutex.RUnlock()

	return c.uncommittedTxMetadataChangesByID.Get(txID)
}

// DeleteAndReturnTxMetadataChangesBySlot deletes all the transaction metadata changes of a certain slot and returns them.
func (c *transactionRetainerCache) DeleteAndReturnTxMetadataChangesBySlot(targetSlot iotago.SlotIndex) map[iotago.TransactionID]*TransactionMetadata {
	c.accessMutex.Lock()
	defer c.accessMutex.Unlock()

	var slots []iotago.SlotIndex

	// iterate over all slots, to ensure that all slots below the given slot are deleted from the cache as well
	c.uncommittedTxIDsBySlot.ForEachKey(func(slot iotago.SlotIndex) bool {
		if slot <= targetSlot {
			slots = append(slots, slot)
		}

		return true
	})

	txIDs := make(map[iotago.TransactionID]struct{})

	for _, slot := range slots {
		// get the affected transaction IDs for the slot and delete the slot from the cache
		slotTxIDs, exists := c.uncommittedTxIDsBySlot.DeleteAndReturn(slot)
		if !exists {
			// no transaction metadata changes for the slot found
			continue
		}

		// add the affected transaction IDs to the list
		for txID := range slotTxIDs {
			txIDs[txID] = struct{}{}
		}
	}

	// get all affected transaction metadata changes and delete them from the cache
	txMetadataChanges := make(map[iotago.TransactionID]*TransactionMetadata, len(txIDs))
	for txID := range txIDs {
		txMeta, exists := c.uncommittedTxMetadataChangesByID.DeleteAndReturn(txID)
		if !exists {
			// transaction metadata not found
			panic(ierrors.Errorf("transaction metadata not found for transaction ID %s", txID.ToHex()))
		}

		txMetadataChanges[txID] = txMeta
	}

	return txMetadataChanges
}
