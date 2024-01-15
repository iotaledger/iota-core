package retainer

import (
	"sync"

	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/iota-core/pkg/storage/prunable/slotstore"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

type metadataStore struct {
	store StoreFunc

	// transaction metadata is kept in one place and stored under the latest attachment slot to align block and transaction pruning
	transactionLatestAttachmentSlot *shrinkingmap.ShrinkingMap[iotago.TransactionID, iotago.SlotIndex]
	attachmentSlotMutex             sync.Mutex
}

func newMetadataStore(store StoreFunc) *metadataStore {
	return &metadataStore{
		store:                           store,
		transactionLatestAttachmentSlot: shrinkingmap.New[iotago.TransactionID, iotago.SlotIndex](),
	}
}

func (m *metadataStore) storeBlockData(blockID iotago.BlockID, failureReason api.BlockFailureReason, transactionID iotago.TransactionID) error {
	store, err := m.store(blockID.Slot())
	if err != nil {
		return ierrors.Wrapf(err, "could not get retainer store for slot %d", blockID.Slot())
	}

	err = m.moveTransactionMetadataToTheLatestAttachmentSlot(transactionID, blockID.Slot())
	if err != nil {
		return err
	}

	if failureReason == api.BlockFailureNone {
		return store.StoreBlockAttached(blockID, transactionID)
	}

	return store.StoreBlockFailure(blockID, failureReason, transactionID)
}

func (m *metadataStore) setBlockAccepted(blockID iotago.BlockID) error {
	store, err := m.store(blockID.Slot())
	if err != nil {
		return ierrors.Wrapf(err, "could not get retainer store for slot %d", blockID.Slot())
	}

	return store.StoreBlockAccepted(blockID)
}

func (m *metadataStore) setBlockConfirmed(blockID iotago.BlockID) error {
	store, err := m.store(blockID.Slot())
	if err != nil {
		return ierrors.Wrapf(err, "could not get retainer store for slot %d", blockID.Slot())
	}

	txID, err := store.StoreBlockConfirmed(blockID)
	if err != nil {
		return err
	}

	if txID != iotago.EmptyTransactionID {
		err = m.updateConfirmedTransaction(txID, blockID.Slot())
		if err != nil {
			return err
		}
	}

	return nil
}

func (m *metadataStore) moveTransactionMetadataToTheLatestAttachmentSlot(txID iotago.TransactionID, latestAttachmentSlot iotago.SlotIndex) error {
	m.attachmentSlotMutex.Lock()
	defer m.attachmentSlotMutex.Unlock()

	if txID == iotago.EmptyTransactionID {
		return nil
	}

	currentSlot, exists := m.transactionLatestAttachmentSlot.Get(txID)
	if !exists {
		m.transactionLatestAttachmentSlot.Set(txID, latestAttachmentSlot)

		return nil
	}

	if currentSlot < latestAttachmentSlot {
		moved, err := m.moveTransactionData(txID, currentSlot, latestAttachmentSlot)
		if moved {
			m.transactionLatestAttachmentSlot.Set(txID, latestAttachmentSlot)
		}
		if err != nil {
			return err
		}
	}

	return nil
}

func (m *metadataStore) moveTransactionData(txID iotago.TransactionID, prevSlot iotago.SlotIndex, newSlot iotago.SlotIndex) (bool, error) {
	store, err := m.store(prevSlot)
	if err != nil {
		return false, ierrors.Wrapf(err, "could not get retainer store for slot %d", prevSlot)
	}

	txData, exist := store.GetTransaction(txID)
	if !exist {
		// nothing to move
		return false, nil
	}

	newStore, err := m.store(newSlot)
	if err != nil {
		return false, ierrors.Wrapf(err, "could not get retainer store for slot %d", newSlot)
	}

	err = newStore.StoreTransactionData(txID, txData)
	if err != nil {
		return false, err
	}

	err = store.DeleteTransactionData(txID)
	if err != nil {
		return true, err
	}

	return true, nil
}

func (m *metadataStore) getBlockData(blockID iotago.BlockID) (*slotstore.BlockRetainerData, error) {
	store, err := m.store(blockID.Slot())
	if err != nil {
		return nil, err
	}

	data, found := store.GetBlock(blockID)
	if !found {
		return nil, ierrors.Errorf("block %s not found", blockID.String())
	}

	return data, nil
}

func (m *metadataStore) getTransactionStoreNoLock(txID iotago.TransactionID) (*slotstore.Retainer, error) {
	latestAttachmentSlot, exists := m.transactionLatestAttachmentSlot.Get(txID)
	if !exists {
		return nil, ierrors.Errorf("latest attachment slot for transaction %s not found", txID.String())
	}

	store, err := m.store(latestAttachmentSlot)
	if err != nil {
		return nil, err
	}

	return store, nil
}

func (m *metadataStore) getTransactionData(txID iotago.TransactionID) (*slotstore.TransactionRetainerData, error) {
	m.attachmentSlotMutex.Lock()
	defer m.attachmentSlotMutex.Unlock()

	store, err := m.getTransactionStoreNoLock(txID)
	if err != nil {
		return nil, err
	}

	data, found := store.GetTransaction(txID)
	if !found {
		return nil, ierrors.Errorf("transaction %s not found", txID.String())
	}

	return data, nil
}

func (m *metadataStore) setTransactionNoFailure(txID iotago.TransactionID, status api.TransactionState) error {
	m.attachmentSlotMutex.Lock()
	defer m.attachmentSlotMutex.Unlock()

	store, err := m.getTransactionStoreNoLock(txID)
	if err != nil {
		return err
	}

	return store.StoreTransactionNoFailure(txID, status)
}

func (m *metadataStore) setTransactionFailure(txID iotago.TransactionID, failureType api.TransactionFailureReason) error {
	m.attachmentSlotMutex.Lock()
	defer m.attachmentSlotMutex.Unlock()

	store, err := m.getTransactionStoreNoLock(txID)
	if err != nil {
		return err
	}

	return store.StoreTransactionFailure(txID, failureType)
}

func (m *metadataStore) updateConfirmedTransaction(txID iotago.TransactionID, attachmentSlot iotago.SlotIndex) error {
	m.attachmentSlotMutex.Lock()
	defer m.attachmentSlotMutex.Unlock()

	store, err := m.getTransactionStoreNoLock(txID)
	if err != nil {
		return err
	}

	data, found := store.GetTransaction(txID)
	if !found {
		return ierrors.Errorf("transaction %s not found", txID.String())
	}

	data.State = api.TransactionStateConfirmed
	data.ConfirmedAttachmentSlot = attachmentSlot

	return store.StoreTransactionData(txID, data)
}
