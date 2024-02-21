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
	store SlotStoreFunc

	// transaction metadata is kept in one place and stored under the latest attachment slot to align block and transaction pruning
	transactionLatestAttachmentSlot *shrinkingmap.ShrinkingMap[iotago.TransactionID, iotago.SlotIndex]
	attachmentSlotMutex             sync.Mutex
}

func newMetadataStore(store SlotStoreFunc) *metadataStore {
	return &metadataStore{
		store:                           store,
		transactionLatestAttachmentSlot: shrinkingmap.New[iotago.TransactionID, iotago.SlotIndex](),
	}
}

func (m *metadataStore) setBlockBooked(blockID iotago.BlockID, transactionID iotago.TransactionID) error {
	store, err := m.store(blockID.Slot())
	if err != nil {
		return ierrors.Wrapf(err, "could not get retainer store for slot %d", blockID.Slot())
	}

	return store.StoreBlockBooked(blockID, transactionID)
}

func (m *metadataStore) setBlockAccepted(blockID iotago.BlockID) error {
	store, err := m.store(blockID.Slot())
	if err != nil {
		return ierrors.Wrapf(err, "could not get retainer store for slot %d", blockID.Slot())
	}

	return store.StoreBlockAccepted(blockID)
}

func (m *metadataStore) setBlockDropped(blockID iotago.BlockID) error {
	store, err := m.store(blockID.Slot())
	if err != nil {
		return ierrors.Wrapf(err, "could not get retainer store for slot %d", blockID.Slot())
	}

	return store.StoreBlockDropped(blockID)
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

func (m *metadataStore) getTransactionStoreWithoutLocking(txID iotago.TransactionID) (*slotstore.Retainer, error) {
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

	store, err := m.getTransactionStoreWithoutLocking(txID)
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

	store, err := m.getTransactionStoreWithoutLocking(txID)
	if err != nil {
		return err
	}

	return store.StoreTransactionNoFailure(txID, status)
}

func (m *metadataStore) setTransactionFailure(txID iotago.TransactionID, failureType api.TransactionFailureReason) error {
	m.attachmentSlotMutex.Lock()
	defer m.attachmentSlotMutex.Unlock()

	store, err := m.getTransactionStoreWithoutLocking(txID)
	if err != nil {
		return err
	}

	return store.StoreTransactionFailure(txID, failureType)
}

func (m *metadataStore) updateConfirmedTransaction(txID iotago.TransactionID, attachmentSlot iotago.SlotIndex) error {
	m.attachmentSlotMutex.Lock()
	defer m.attachmentSlotMutex.Unlock()

	store, err := m.getTransactionStoreWithoutLocking(txID)
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
