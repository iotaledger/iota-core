package prunable

import (
	"github.com/iotaledger/hive.go/core/storable"
	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/nodeclient/apimodels"
)

const (
	blockOrphanedPrefix byte = iota
	blockConfirmedPrefix
	transactionPendingPrefix
	transactionConfirmedPrefix
	blockFailurePrefix
	transactionFailurePrefix
)

type Retainer struct {
	slot                      iotago.SlotIndex
	blockOrphanedStore        *kvstore.TypedStore[iotago.BlockID, types.Empty]
	blockConfirmedStore       *kvstore.TypedStore[iotago.BlockID, types.Empty]
	transactionPendingStore   *kvstore.TypedStore[iotago.BlockID, types.Empty]
	transactionConfirmedStore *kvstore.TypedStore[iotago.BlockID, types.Empty]
	blockFailureStore         *kvstore.TypedStore[iotago.BlockID, storable.SerializableInt64]
	transactionFailureStore   *kvstore.TypedStore[iotago.TransactionID, storable.SerializableInt64]
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
		blockFailureStore: kvstore.NewTypedStore(lo.PanicOnErr(store.WithExtendedRealm(kvstore.Realm{blockFailurePrefix})),
			iotago.SlotIdentifier.Bytes,
			iotago.SlotIdentifierFromBytes,
			storable.SerializableInt64.Bytes,
			func(bytes []byte) (storable.SerializableInt64, int, error) {
				var i storable.SerializableInt64
				c, err := i.FromBytes(bytes)

				return i, c, err
			}),
		transactionFailureStore: kvstore.NewTypedStore(lo.PanicOnErr(store.WithExtendedRealm(kvstore.Realm{transactionFailurePrefix})),
			iotago.TransactionID.Bytes,
			iotago.IdentifierFromBytes,
			storable.SerializableInt64.Bytes,
			func(bytes []byte) (storable.SerializableInt64, int, error) {
				var i storable.SerializableInt64
				c, err := i.FromBytes(bytes)

				return i, c, err
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

func (r *Retainer) StoreBlockFailure(blockID iotago.BlockID, failureType apimodels.BlockFailureReason) error {
	err := r.blockFailureStore.Set(blockID, storable.SerializableInt64(failureType))
	if err != nil {
		return err
	}

	return nil
}

func (r *Retainer) StoreTransactionFailure(transactionID iotago.TransactionID, failureType apimodels.TransactionFailureReason) error {
	err := r.transactionFailureStore.Set(transactionID, storable.SerializableInt64(failureType))
	if err != nil {
		return err
	}

	return nil
}
