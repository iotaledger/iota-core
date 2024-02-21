package retainer

import (
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/iota-core/pkg/storage/prunable/slotstore"
	iotago "github.com/iotaledger/iota.go/v4"
)

type metadataStore struct {
	store SlotStoreFunc
}

func newMetadataStore(store SlotStoreFunc) *metadataStore {
	return &metadataStore{
		store: store,
	}
}

func (m *metadataStore) setBlockBooked(blockID iotago.BlockID) error {
	store, err := m.store(blockID.Slot())
	if err != nil {
		return ierrors.Wrapf(err, "could not get retainer store for slot %d", blockID.Slot())
	}

	return store.StoreBlockBooked(blockID)
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

	if err := store.StoreBlockConfirmed(blockID); err != nil {
		return err
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
