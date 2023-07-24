package prunable

import (
	"github.com/iotaledger/hive.go/core/storable"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/serializer/v2/marshalutil"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/nodeclient/apimodels"
)

const (
	blockStorePrefix byte = iota
	transactionStorePrefix
)

type BlockRetainerData struct {
	State         apimodels.BlockState
	FailureReason apimodels.BlockFailureReason
}

func (b *BlockRetainerData) Bytes() ([]byte, error) {
	marshalUtil := marshalutil.New()
	marshalUtil.WriteUint8(uint8(b.State))
	marshalUtil.WriteUint8(uint8(b.FailureReason))
	return marshalUtil.Bytes(), nil
}

func (b *BlockRetainerData) FromBytes(bytes []byte) (int, error) {
	marshalUtil := marshalutil.New(bytes)
	state, err := marshalUtil.ReadUint8()
	if err != nil {
		return 0, err
	}
	b.State = apimodels.BlockState(state)
	reason, err := marshalUtil.ReadUint8()
	if err != nil {
		return 0, err
	}
	b.FailureReason = apimodels.BlockFailureReason(reason)
	return marshalUtil.ReadOffset(), nil
}

type TransactionRetainerData struct {
	State         apimodels.TransactionState
	FailureReason apimodels.TransactionFailureReason
}

func (t *TransactionRetainerData) Bytes() ([]byte, error) {
	marshalUtil := marshalutil.New()
	marshalUtil.WriteUint8(uint8(t.State))
	marshalUtil.WriteUint8(uint8(t.FailureReason))
	return marshalUtil.Bytes(), nil
}

func (t *TransactionRetainerData) FromBytes(bytes []byte) (int, error) {
	marshalUtil := marshalutil.New(bytes)
	state, err := marshalUtil.ReadUint8()
	if err != nil {
		return 0, err
	}
	t.State = apimodels.TransactionState(state)
	reason, err := marshalUtil.ReadUint8()
	if err != nil {
		return 0, err
	}
	t.FailureReason = apimodels.TransactionFailureReason(reason)
	return marshalUtil.ReadOffset(), nil
}

type Retainer struct {
	slot       iotago.SlotIndex
	blockStore *kvstore.TypedStore[iotago.BlockID, *BlockRetainerData]
	// we store transaction metadata per blockID as in API requests we always request by blockID
	transactionStore *kvstore.TypedStore[iotago.BlockID, *TransactionRetainerData]
}

func NewRetainer(slot iotago.SlotIndex, store kvstore.KVStore) (newRetainer *Retainer) {
	// core.BlockIDFromTransactionID()
	return &Retainer{
		slot: slot,
		blockStore: kvstore.NewTypedStore(lo.PanicOnErr(store.WithExtendedRealm(kvstore.Realm{blockStorePrefix})),
			iotago.SlotIdentifier.Bytes,
			iotago.SlotIdentifierFromBytes,
			(*BlockRetainerData).Bytes,
			func(bytes []byte) (*BlockRetainerData, int, error) {
				b := new(BlockRetainerData)
				c, err := b.FromBytes(bytes)

				return b, c, err
			},
		),
		transactionStore: kvstore.NewTypedStore(lo.PanicOnErr(store.WithExtendedRealm(kvstore.Realm{transactionStorePrefix})),
			iotago.SlotIdentifier.Bytes,
			iotago.SlotIdentifierFromBytes,
			(*TransactionRetainerData).Bytes,
			func(bytes []byte) (*TransactionRetainerData, int, error) {
				t := new(TransactionRetainerData)
				c, err := t.FromBytes(bytes)

				return t, c, err
			},
		),
	}
}

func (r *Retainer) StoreBlockAttached(blockID iotago.BlockID) error {
	if err := r.blockStore.Set(blockID, &BlockRetainerData{
		State:         apimodels.BlockStatePending,
		FailureReason: apimodels.NoBlockFailureReason,
	}); err != nil {
		return err
	}
	//
	//if !transactionID.Empty() {
	//	if err := r.transactionStore.Set(transactionID, TransactionRetainerData{
	//		State:         apimodels.TransactionStatePending,
	//		FailureReason: apimodels.NoTransactionFailureReason,
	//	}); err != nil {
	//		return ierrors.Errorf("failed to retain transaction in pending store: %w", err)
	//	}
	//}

	return nil
}

func (r *Retainer) GetBlock(blockID iotago.BlockID) (*BlockRetainerData, bool) {
	blockData, err := r.blockStore.Get(blockID)
	if err != nil {
		return nil, false
	}

	return blockData, true
}

func (r *Retainer) GetTransaction(blockID iotago.BlockID) (*TransactionRetainerData, bool) {
	txData, err := r.transactionStore.Get(blockID)
	if err != nil {
		return nil, false
	}

	return txData, true
}

func (r *Retainer) StoreBlockAccepted(blockID iotago.BlockID) error {
	err := r.blockStore.Set(blockID, &BlockRetainerData{
		State:         apimodels.BlockStateAccepted,
		FailureReason: apimodels.NoBlockFailureReason,
	})
	if err != nil {
		return err
	}

	return nil
}

func (r *Retainer) StoreBlockConfirmed(blockID iotago.BlockID) error {
	err := r.blockStore.Set(blockID, &BlockRetainerData{
		State:         apimodels.BlockStateConfirmed,
		FailureReason: apimodels.NoBlockFailureReason,
	})
	if err != nil {
		return err
	}

	return nil
}

func (r *Retainer) StoreTransactionPending(blockID iotago.BlockID) error {
	if err := r.transactionStore.Set(blockID, &TransactionRetainerData{
		State:         apimodels.TransactionStatePending,
		FailureReason: apimodels.NoTransactionFailureReason,
	}); err != nil {
		return nil
	}
}

func (r *Retainer) StoreTransactionNoFailureStatus(blockID iotago.BlockID, status apimodels.TransactionState) error {
	if status == apimodels.TransactionStateFailed {
		return ierrors.Errorf("failed to retain transaction status, status cannot be failed, blockID: %s", blockID.String())
	}

	err := r.transactionStore.Set(blockID, &TransactionRetainerData{
		State:         status,
		FailureReason: apimodels.NoTransactionFailureReason,
	})
	if err != nil {
		return err
	}

	return nil
}

func (r *Retainer) DeleteTransactionData(prevID iotago.BlockID) error {
	err := r.transactionStore.Delete(prevID)
	if err != nil {
		return err
	}

	return nil
}

func (r *Retainer) StoreBlockFailure(blockID iotago.BlockID, failureType apimodels.BlockFailureReason) error {
	err := r.blockStore.Set(blockID, storable.SerializableInt64(failureType))
	if err != nil {
		return err
	}

	return nil
}

func (r *Retainer) StoreTransactionFailure(transactionID iotago.TransactionID, failureType apimodels.TransactionFailureReason) error {
	err := r.transactionStore.Set(transactionID, storable.SerializableInt64(failureType))
	if err != nil {
		return err
	}

	return nil
}
