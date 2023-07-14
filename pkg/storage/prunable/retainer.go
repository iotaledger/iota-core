package prunable

import (
	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
)

const (
	orphanedPrefix byte = iota
	confirmedPrefix
)

type BlockStatus int

const (
	Unknown BlockStatus = iota
	Accepted
	Confirmed
	Orphaned
)

type Retainer struct {
	slot           iotago.SlotIndex
	orphanedStore  *kvstore.TypedStore[iotago.BlockID, types.Empty]
	confirmedStore *kvstore.TypedStore[iotago.BlockID, types.Empty]
}

func NewRetainer(slot iotago.SlotIndex, store kvstore.KVStore) (newRetainer *Retainer) {
	return &Retainer{
		slot: slot,
		orphanedStore: kvstore.NewTypedStore[iotago.BlockID, types.Empty](lo.PanicOnErr(store.WithExtendedRealm(kvstore.Realm{orphanedPrefix})),
			iotago.SlotIdentifier.Bytes,
			iotago.SlotIdentifierFromBytes,
			types.Empty.Bytes,
			func(bytes []byte) (object types.Empty, consumed int, err error) {
				return types.Void, 0, nil
			}),
		confirmedStore: kvstore.NewTypedStore[iotago.BlockID, types.Empty](lo.PanicOnErr(store.WithExtendedRealm(kvstore.Realm{confirmedPrefix})),
			iotago.SlotIdentifier.Bytes,
			iotago.SlotIdentifierFromBytes,
			types.Empty.Bytes,
			func(bytes []byte) (object types.Empty, consumed int, err error) {
				return types.Void, 0, nil
			}),
	}
}

func (r *Retainer) Store(blockID iotago.BlockID) error {
	err := r.orphanedStore.Set(blockID, types.Void)
	if err != nil {
		return err
	}

	return nil
}

func (r *Retainer) Load(blockID iotago.BlockID) (BlockStatus, error) {
	exists, err := r.confirmedStore.Has(blockID)
	if err != nil {
		return 0, err
	}
	if exists {
		return Confirmed, nil
	}

	exists, err = r.orphanedStore.Has(blockID)
	if err != nil {
		return 0, err
	}
	if exists {
		return Orphaned, nil
	}

	return Unknown, nil
}

func (r *Retainer) StoreAccepted(blockID iotago.BlockID) error {
	err := r.orphanedStore.Delete(blockID)
	if err != nil {
		return err
	}
	return nil
}

func (r *Retainer) StoreConfirmed(blockID iotago.BlockID) error {
	err := r.confirmedStore.Set(blockID, types.Void)
	if err != nil {
		return err
	}
	return nil
}
