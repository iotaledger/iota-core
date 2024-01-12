package slotstore

import (
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/serializer/v2/stream"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

const (
	blockStorePrefix byte = iota
	transactionStorePrefix
)

type BlockRetainerData struct {
	State         api.BlockState
	FailureReason api.BlockFailureReason
	TransactionID iotago.TransactionID
}

func (b *BlockRetainerData) Bytes() ([]byte, error) {
	byteBuffer := stream.NewByteBuffer(2)

	if err := stream.Write(byteBuffer, b.State); err != nil {
		return nil, ierrors.Wrap(err, "failed to write block state")
	}
	if err := stream.Write(byteBuffer, b.FailureReason); err != nil {
		return nil, ierrors.Wrap(err, "failed to write block failure reason")
	}

	if err := stream.Write(byteBuffer, b.TransactionID); err != nil {
		return nil, ierrors.Wrap(err, "failed to write transaction ID")
	}

	return byteBuffer.Bytes()
}

func blockRetainerDataFromBytes(bytes []byte) (*BlockRetainerData, int, error) {
	byteReader := stream.NewByteReader(bytes)

	var err error
	b := new(BlockRetainerData)

	if b.State, err = stream.Read[api.BlockState](byteReader); err != nil {
		return nil, 0, ierrors.Wrap(err, "failed to read block state")
	}
	if b.FailureReason, err = stream.Read[api.BlockFailureReason](byteReader); err != nil {
		return nil, 0, ierrors.Wrap(err, "failed to read block failure reason")
	}
	if b.TransactionID, err = stream.Read[iotago.TransactionID](byteReader); err != nil {
		return nil, 0, ierrors.Wrap(err, "failed to read transaction ID")
	}

	return b, byteReader.BytesRead(), nil
}

type TransactionRetainerData struct {
	State         api.TransactionState
	FailureReason api.TransactionFailureReason
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

func transactionRetainerDataFromBytes(bytes []byte) (*TransactionRetainerData, int, error) {
	byteReader := stream.NewByteReader(bytes)

	var err error
	t := new(TransactionRetainerData)

	if t.State, err = stream.Read[api.TransactionState](byteReader); err != nil {
		return nil, 0, ierrors.Wrap(err, "failed to read transaction state")
	}
	if t.FailureReason, err = stream.Read[api.TransactionFailureReason](byteReader); err != nil {
		return nil, 0, ierrors.Wrap(err, "failed to read transaction failure reason")
	}

	return t, byteReader.BytesRead(), nil
}

type Retainer struct {
	slot       iotago.SlotIndex
	blockStore *kvstore.TypedStore[iotago.BlockID, *BlockRetainerData]
	// we store transaction metadata per blockID as in API requests we always request by blockID
	transactionStore *kvstore.TypedStore[iotago.TransactionID, *TransactionRetainerData]
}

func NewRetainer(slot iotago.SlotIndex, store kvstore.KVStore) (newRetainer *Retainer) {
	return &Retainer{
		slot: slot,
		blockStore: kvstore.NewTypedStore(lo.PanicOnErr(store.WithExtendedRealm(kvstore.Realm{blockStorePrefix})),
			iotago.BlockID.Bytes,
			iotago.BlockIDFromBytes,
			(*BlockRetainerData).Bytes,
			blockRetainerDataFromBytes,
		),
		transactionStore: kvstore.NewTypedStore(lo.PanicOnErr(store.WithExtendedRealm(kvstore.Realm{transactionStorePrefix})),
			iotago.TransactionID.Bytes,
			iotago.TransactionIDFromBytes,
			(*TransactionRetainerData).Bytes,
			transactionRetainerDataFromBytes,
		),
	}
}

func (r *Retainer) StoreBlockAttached(blockID iotago.BlockID) error {
	return r.blockStore.Set(blockID, &BlockRetainerData{
		State:         api.BlockStatePending,
		FailureReason: api.BlockFailureNone,
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
		State:         api.BlockStateAccepted,
		FailureReason: api.BlockFailureNone,
	})
}

func (r *Retainer) StoreBlockConfirmed(blockID iotago.BlockID) error {
	return r.blockStore.Set(blockID, &BlockRetainerData{
		State:         api.BlockStateConfirmed,
		FailureReason: api.BlockFailureNone,
	})
}

func (r *Retainer) StoreTransactionPending(blockID iotago.BlockID) error {
	return r.transactionStore.Set(blockID, &TransactionRetainerData{
		State:         api.TransactionStatePending,
		FailureReason: api.TxFailureNone,
	})
}

func (r *Retainer) StoreTransactionNoFailureStatus(blockID iotago.BlockID, status api.TransactionState) error {
	if status == api.TransactionStateFailed {
		return ierrors.Errorf("failed to retain transaction status, status cannot be failed, blockID: %s", blockID.String())
	}

	return r.transactionStore.Set(blockID, &TransactionRetainerData{
		State:         status,
		FailureReason: api.TxFailureNone,
	})
}

func (r *Retainer) DeleteTransactionData(prevID iotago.BlockID) error {
	return r.transactionStore.Delete(prevID)
}

func (r *Retainer) StoreBlockFailure(blockID iotago.BlockID, failureType api.BlockFailureReason) error {
	return r.blockStore.Set(blockID, &BlockRetainerData{
		State:         api.BlockStateFailed,
		FailureReason: failureType,
	})
}

func (r *Retainer) StoreTransactionFailure(blockID iotago.BlockID, failureType api.TransactionFailureReason) error {
	return r.transactionStore.Set(blockID, &TransactionRetainerData{
		State:         api.TransactionStateFailed,
		FailureReason: failureType,
	})
}
