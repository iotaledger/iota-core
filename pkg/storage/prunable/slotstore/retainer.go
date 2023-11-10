package slotstore

import (
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/serializer/v2/stream"
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
	byteBuffer := stream.NewByteBuffer(2)

	if err := stream.Write(byteBuffer, b.State); err != nil {
		return nil, ierrors.Wrap(err, "failed to write block state")
	}
	if err := stream.Write(byteBuffer, b.FailureReason); err != nil {
		return nil, ierrors.Wrap(err, "failed to write block failure reason")
	}

	return byteBuffer.Bytes()
}

func BlockRetainerDataFromBytes(bytes []byte) (*BlockRetainerData, int, error) {
	byteReader := stream.NewByteReader(bytes)

	var err error
	b := new(BlockRetainerData)

	if b.State, err = stream.Read[apimodels.BlockState](byteReader); err != nil {
		return nil, 0, ierrors.Wrap(err, "failed to read block state")
	}
	if b.FailureReason, err = stream.Read[apimodels.BlockFailureReason](byteReader); err != nil {
		return nil, 0, ierrors.Wrap(err, "failed to read block failure reason")
	}

	return b, byteReader.BytesRead(), nil
}

type TransactionRetainerData struct {
	State         apimodels.TransactionState
	FailureReason apimodels.TransactionFailureReason
}

func (t *TransactionRetainerData) Bytes() ([]byte, error) {
	byteBuffer := stream.NewByteBuffer(2)

	if err := stream.Write(byteBuffer, t.State); err != nil {
		return nil, ierrors.Wrap(err, "failed to write transaction state")
	}
	if err := stream.Write(byteBuffer, t.FailureReason); err != nil {
		return nil, ierrors.Wrap(err, "failed to write transaction failure reason")
	}

	return byteBuffer.Bytes()
}

func TransactionRetainerDataFromBytes(bytes []byte) (*TransactionRetainerData, int, error) {
	byteReader := stream.NewByteReader(bytes)

	var err error
	t := new(TransactionRetainerData)

	if t.State, err = stream.Read[apimodels.TransactionState](byteReader); err != nil {
		return nil, 0, ierrors.Wrap(err, "failed to read transaction state")
	}
	if t.FailureReason, err = stream.Read[apimodels.TransactionFailureReason](byteReader); err != nil {
		return nil, 0, ierrors.Wrap(err, "failed to read transaction failure reason")
	}

	return t, byteReader.BytesRead(), nil
}

type Retainer struct {
	slot       iotago.SlotIndex
	blockStore *kvstore.TypedStore[iotago.BlockID, *BlockRetainerData]
	// we store transaction metadata per blockID as in API requests we always request by blockID
	transactionStore *kvstore.TypedStore[iotago.BlockID, *TransactionRetainerData]
}

func NewRetainer(slot iotago.SlotIndex, store kvstore.KVStore) (newRetainer *Retainer) {
	return &Retainer{
		slot: slot,
		blockStore: kvstore.NewTypedStore(lo.PanicOnErr(store.WithExtendedRealm(kvstore.Realm{blockStorePrefix})),
			iotago.BlockID.Bytes,
			iotago.BlockIDFromBytes,
			(*BlockRetainerData).Bytes,
			BlockRetainerDataFromBytes,
		),
		transactionStore: kvstore.NewTypedStore(lo.PanicOnErr(store.WithExtendedRealm(kvstore.Realm{transactionStorePrefix})),
			iotago.BlockID.Bytes,
			iotago.BlockIDFromBytes,
			(*TransactionRetainerData).Bytes,
			TransactionRetainerDataFromBytes,
		),
	}
}

func (r *Retainer) StoreBlockAttached(blockID iotago.BlockID) error {
	return r.blockStore.Set(blockID, &BlockRetainerData{
		State:         apimodels.BlockStatePending,
		FailureReason: apimodels.BlockFailureNone,
	})
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
	return r.blockStore.Set(blockID, &BlockRetainerData{
		State:         apimodels.BlockStateAccepted,
		FailureReason: apimodels.BlockFailureNone,
	})
}

func (r *Retainer) StoreBlockConfirmed(blockID iotago.BlockID) error {
	return r.blockStore.Set(blockID, &BlockRetainerData{
		State:         apimodels.BlockStateConfirmed,
		FailureReason: apimodels.BlockFailureNone,
	})
}

func (r *Retainer) StoreTransactionPending(blockID iotago.BlockID) error {
	return r.transactionStore.Set(blockID, &TransactionRetainerData{
		State:         apimodels.TransactionStatePending,
		FailureReason: apimodels.TxFailureNone,
	})
}

func (r *Retainer) StoreTransactionNoFailureStatus(blockID iotago.BlockID, status apimodels.TransactionState) error {
	if status == apimodels.TransactionStateFailed {
		return ierrors.Errorf("failed to retain transaction status, status cannot be failed, blockID: %s", blockID.String())
	}

	return r.transactionStore.Set(blockID, &TransactionRetainerData{
		State:         status,
		FailureReason: apimodels.TxFailureNone,
	})
}

func (r *Retainer) DeleteTransactionData(prevID iotago.BlockID) error {
	return r.transactionStore.Delete(prevID)
}

func (r *Retainer) StoreBlockFailure(blockID iotago.BlockID, failureType apimodels.BlockFailureReason) error {
	return r.blockStore.Set(blockID, &BlockRetainerData{
		State:         apimodels.BlockStateFailed,
		FailureReason: failureType,
	})
}

func (r *Retainer) StoreTransactionFailure(blockID iotago.BlockID, failureType apimodels.TransactionFailureReason) error {
	return r.transactionStore.Set(blockID, &TransactionRetainerData{
		State:         apimodels.TransactionStateFailed,
		FailureReason: failureType,
	})
}
