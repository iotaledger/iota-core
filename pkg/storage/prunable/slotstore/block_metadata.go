package slotstore

import (
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/serializer/v2"
	"github.com/iotaledger/hive.go/serializer/v2/stream"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

const (
	// api.BlockState.
	blockMetadataLength = serializer.OneByte
)

type BlockMetadata struct {
	State api.BlockState
}

func (b *BlockMetadata) Bytes() ([]byte, error) {
	byteBuffer := stream.NewByteBuffer(blockMetadataLength)

	if err := stream.Write(byteBuffer, b.State); err != nil {
		return nil, ierrors.Wrap(err, "failed to write block state")
	}

	return byteBuffer.Bytes()
}

func blockMetadataFromBytes(bytes []byte) (*BlockMetadata, int, error) {
	byteReader := stream.NewByteReader(bytes)

	var err error
	blockMetadata := new(BlockMetadata)

	if blockMetadata.State, err = stream.Read[api.BlockState](byteReader); err != nil {
		return nil, 0, ierrors.Wrap(err, "failed to read block state")
	}

	return blockMetadata, byteReader.BytesRead(), nil
}

type BlockMetadataStore struct {
	// The slot for which this store holds the metadata from all blocks with the corresponding slot.
	slot               iotago.SlotIndex
	blockMetadataStore *kvstore.TypedStore[iotago.BlockID, *BlockMetadata]
}

func NewBlockMetadataStore(slot iotago.SlotIndex, store kvstore.KVStore) *BlockMetadataStore {
	return &BlockMetadataStore{
		slot: slot,
		blockMetadataStore: kvstore.NewTypedStore(store,
			iotago.BlockID.Bytes,
			iotago.BlockIDFromBytes,
			(*BlockMetadata).Bytes,
			blockMetadataFromBytes,
		),
	}
}

func (r *BlockMetadataStore) StoreBlockMetadata(blockID iotago.BlockID, state api.BlockState) error {
	return r.blockMetadataStore.Set(blockID, &BlockMetadata{State: state})
}

func (r *BlockMetadataStore) BlockMetadata(blockID iotago.BlockID) (*BlockMetadata, error) {
	blockMetadata, err := r.blockMetadataStore.Get(blockID)
	if err != nil {
		return nil, err
	}

	return blockMetadata, nil
}
