package slotstore

import (
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/serializer/v2"
	"github.com/iotaledger/hive.go/serializer/v2/stream"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

const (
	blockStorePrefix byte = iota
	transactionStorePrefix

	// api.TransactionState + api.TransactionFailureReason + iotago.SlotIndex
	transactionRetainerDataLength = serializer.OneByte + serializer.OneByte + iotago.SlotIndexLength

	// api.BlockState + api.BlockFailureReason + iotago.TransactionID
	blockRetainerDataLength = serializer.OneByte + serializer.OneByte + iotago.TransactionIDLength
)

type BlockRetainerData struct {
	State         api.BlockState
	FailureReason api.BlockFailureReason
	TransactionID iotago.TransactionID
}

func (b *BlockRetainerData) Bytes() ([]byte, error) {
	byteBuffer := stream.NewByteBuffer(blockRetainerDataLength)

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
	// needed for a finalization status evaluation
	ConfirmedAttachmentSlot iotago.SlotIndex
}

func (t *TransactionRetainerData) Bytes() ([]byte, error) {
	byteBuffer := stream.NewByteBuffer(transactionRetainerDataLength)

	if err := stream.Write(byteBuffer, t.State); err != nil {
		return nil, ierrors.Wrap(err, "failed to write transaction state")
	}

	if err := stream.Write(byteBuffer, t.FailureReason); err != nil {
		return nil, ierrors.Wrap(err, "failed to write transaction failure reason")
	}

	if err := stream.Write(byteBuffer, t.ConfirmedAttachmentSlot); err != nil {
		return nil, ierrors.Wrap(err, "failed to write confirmed attachment slot")
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

	if t.ConfirmedAttachmentSlot, err = stream.Read[iotago.SlotIndex](byteReader); err != nil {
		return nil, 0, ierrors.Wrap(err, "failed to read confirmed attachment slot")
	}

	return t, byteReader.BytesRead(), nil
}

type SlotStore struct {
	slot       iotago.SlotIndex
	blockStore *kvstore.TypedStore[iotago.BlockID, *BlockRetainerData]
	// we store transaction metadata per blockID as in API requests we always request by blockID
	transactionStore *kvstore.TypedStore[iotago.TransactionID, *TransactionRetainerData]
}

func NewSlotStore(slot iotago.SlotIndex, store kvstore.KVStore) (newSlotStore *SlotStore) {
	return &SlotStore{
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

func (s *SlotStore) StoreBlockBooked(blockID iotago.BlockID, transactionID iotago.TransactionID) error {
	return s.blockStore.Set(blockID, &BlockRetainerData{
		State:         api.BlockStatePending,
		FailureReason: api.BlockFailureNone,
		TransactionID: transactionID,
	})
}

func (s *SlotStore) StoreBlockFailure(blockID iotago.BlockID, failureType api.BlockFailureReason, transactionID iotago.TransactionID) error {
	return s.blockStore.Set(blockID, &BlockRetainerData{
		State:         api.BlockStateFailed,
		FailureReason: failureType,
		TransactionID: transactionID,
	})
}

func (s *SlotStore) StoreBlockAccepted(blockID iotago.BlockID) error {
	data, err := s.blockStore.Get(blockID)
	if err != nil {
		return err
	}

	data.State = api.BlockStateAccepted
	data.FailureReason = api.BlockFailureNone

	return s.blockStore.Set(blockID, data)
}

func (s *SlotStore) StoreBlockConfirmed(blockID iotago.BlockID) (iotago.TransactionID, error) {
	data, err := s.blockStore.Get(blockID)
	if err != nil {
		return iotago.EmptyTransactionID, err
	}

	data.State = api.BlockStateConfirmed
	data.FailureReason = api.BlockFailureNone

	return data.TransactionID, s.blockStore.Set(blockID, data)
}

func (s *SlotStore) StoreBlockDropped(blockID iotago.BlockID) error {
	data, err := s.blockStore.Get(blockID)
	if err != nil {
		return err
	}

	data.State = api.BlockStateFailed
	data.FailureReason = api.BlockFailureDroppedDueToCongestion

	return s.blockStore.Set(blockID, data)
}

func (s *SlotStore) GetBlock(blockID iotago.BlockID) (*BlockRetainerData, bool) {
	blockData, err := s.blockStore.Get(blockID)
	if err != nil {
		return nil, false
	}

	return blockData, true
}

func (s *SlotStore) StoreTransactionData(transactionID iotago.TransactionID, data *TransactionRetainerData) error {
	return s.transactionStore.Set(transactionID, data)
}

func (s *SlotStore) StoreTransactionNoFailure(transactionID iotago.TransactionID, status api.TransactionState) error {
	if status == api.TransactionStateFailed {
		return ierrors.Errorf("failed to retain transaction status, status cannot be failed, transactionID: %s", transactionID.String())
	}

	return s.transactionStore.Set(transactionID, &TransactionRetainerData{
		State:         status,
		FailureReason: api.TxFailureNone,
	})
}

func (s *SlotStore) StoreTransactionFailure(transactionID iotago.TransactionID, failureType api.TransactionFailureReason) error {
	return s.transactionStore.Set(transactionID, &TransactionRetainerData{
		State:         api.TransactionStateFailed,
		FailureReason: failureType,
	})
}

func (s *SlotStore) DeleteTransactionData(prevID iotago.TransactionID) error {
	return s.transactionStore.Delete(prevID)
}

func (s *SlotStore) GetTransaction(txID iotago.TransactionID) (*TransactionRetainerData, bool) {
	txData, err := s.transactionStore.Get(txID)
	if err != nil {
		return nil, false
	}

	return txData, true
}
