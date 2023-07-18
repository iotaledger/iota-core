package prunable

import (
	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
)

const (
	blockOrphanedPrefix byte = iota
	blockConfirmedPrefix
	transactionPendingPrefix
	transactionConfirmedPrefix
)

type Retainer struct {
	slot                      iotago.SlotIndex
	blockOrphanedStore        *kvstore.TypedStore[iotago.BlockID, types.Empty]
	blockConfirmedStore       *kvstore.TypedStore[iotago.BlockID, types.Empty]
	transactionPendingStore   *kvstore.TypedStore[iotago.BlockID, types.Empty]
	transactionConfirmedStore *kvstore.TypedStore[iotago.BlockID, types.Empty]
}

func NewRetainer(slot iotago.SlotIndex, store kvstore.KVStore) (newRetainer *Retainer) {
	return &Retainer{
		slot: slot,
		blockOrphanedStore: kvstore.NewTypedStore(lo.PanicOnErr(store.WithExtendedRealm(kvstore.Realm{blockOrphanedPrefix})),
			iotago.SlotIdentifier.Bytes,
			iotago.SlotIdentifierFromBytes,
			types.Empty.Bytes,
			func(bytes []byte) (object types.Empty, consumed int, err error) {
				return types.Void, 0, nil
			}),
		blockConfirmedStore: kvstore.NewTypedStore(lo.PanicOnErr(store.WithExtendedRealm(kvstore.Realm{blockConfirmedPrefix})),
			iotago.SlotIdentifier.Bytes,
			iotago.SlotIdentifierFromBytes,
			types.Empty.Bytes,
			func(bytes []byte) (object types.Empty, consumed int, err error) {
				return types.Void, 0, nil
			}),
		transactionPendingStore: kvstore.NewTypedStore(lo.PanicOnErr(store.WithExtendedRealm(kvstore.Realm{transactionPendingPrefix})),
			iotago.SlotIdentifier.Bytes,
			iotago.SlotIdentifierFromBytes,
			types.Empty.Bytes,
			func(bytes []byte) (object types.Empty, consumed int, err error) {
				return types.Void, 0, nil
			}),
		transactionConfirmedStore: kvstore.NewTypedStore(lo.PanicOnErr(store.WithExtendedRealm(kvstore.Realm{transactionConfirmedPrefix})),
			iotago.SlotIdentifier.Bytes,
			iotago.SlotIdentifierFromBytes,
			types.Empty.Bytes,
			func(bytes []byte) (object types.Empty, consumed int, err error) {
				return types.Void, 0, nil
			}),
	}
}

func (r *Retainer) Store(blockID iotago.BlockID, hasTx bool) error {
	err := r.blockOrphanedStore.Set(blockID, types.Void)
	if err != nil {
		return err
	}

	if hasTx {
		err2 := r.transactionPendingStore.Set(blockID, types.Void)
		if err2 != nil {
			return ierrors.Errorf("failed to retain transaction in pending store: %w", err)
		}
	}

	return nil
}

func (r *Retainer) WasBlockConfirmed(blockID iotago.BlockID) (bool, error) {
	exists, err := r.blockConfirmedStore.Has(blockID)
	if err != nil {
		return false, err
	}

	return exists, nil
}

func (r *Retainer) WasBlockOrphaned(blockID iotago.BlockID) (bool, error) {
	exists, err := r.blockOrphanedStore.Has(blockID)
	if err != nil {
		return false, err
	}

	return exists, nil
}

func (r *Retainer) StoreBlockAccepted(blockID iotago.BlockID) error {
	err := r.blockOrphanedStore.Delete(blockID)
	if err != nil {
		return err
	}

	return nil
}

func (r *Retainer) StoreBlockConfirmed(blockID iotago.BlockID) error {
	err := r.blockConfirmedStore.Set(blockID, types.Void)
	if err != nil {
		return err
	}

	return nil
}

func (r *Retainer) WasTransactionConfirmed(blockID iotago.BlockID) (bool, error) {
	exists, err := r.transactionConfirmedStore.Has(blockID)
	if err != nil {
		return false, err
	}

	return exists, nil
}

func (r *Retainer) WasTransactionPending(blockID iotago.BlockID) (bool, error) {
	exists, err := r.transactionPendingStore.Has(blockID)
	if err != nil {
		return false, err
	}

	return exists, nil
}

func (r *Retainer) StoreTransactionPending(blockID iotago.BlockID) error {
	err := r.transactionPendingStore.Set(blockID, types.Void)
	if err != nil {
		return err
	}

	return nil
}

func (r *Retainer) StoreTransactionConfirmed(blockID iotago.BlockID) error {
	err := r.transactionConfirmedStore.Set(blockID, types.Void)
	if err != nil {
		return err
	}

	return nil
}

func (r *Retainer) DeleteTransactionConfirmed(prevID iotago.BlockID) error {
	err := r.transactionConfirmedStore.Delete(prevID)
	if err != nil {
		return err
	}

	return nil
}
