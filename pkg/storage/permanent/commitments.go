package permanent

import (
	"io"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/serializer/v2/stream"
	"github.com/iotaledger/iota-core/pkg/model"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Commitments struct {
	apiProvider iotago.APIProvider
	store       *kvstore.TypedStore[iotago.SlotIndex, *model.Commitment]
}

func NewCommitments(store kvstore.KVStore, apiProvider iotago.APIProvider) *Commitments {
	return &Commitments{
		apiProvider: apiProvider,
		store: kvstore.NewTypedStore(store,
			iotago.SlotIndex.Bytes,
			iotago.SlotIndexFromBytes,
			func(c *model.Commitment) ([]byte, error) {
				return c.Data(), nil
			},
			func(bytes []byte) (*model.Commitment, int, error) {
				c, err := model.CommitmentFromBytes(bytes, apiProvider)
				return c, len(bytes), err
			},
		),
	}
}

func (c *Commitments) Store(commitment *model.Commitment) error {
	return c.store.Set(commitment.Commitment().Slot, commitment)
}

func (c *Commitments) Load(slot iotago.SlotIndex) (commitment *model.Commitment, err error) {
	return c.store.Get(slot)
}

func (c *Commitments) Export(writer io.WriteSeeker, targetSlot iotago.SlotIndex) (err error) {
	if err := stream.WriteCollection(writer, func() (elementsCount uint64, err error) {
		var count uint64
		for slot := c.apiProvider.CommittedAPI().ProtocolParameters().GenesisSlot(); slot <= targetSlot; slot++ {
			commitmentBytes, err := c.store.KVStore().Get(lo.PanicOnErr(slot.Bytes()))
			if err != nil {
				return 0, ierrors.Wrapf(err, "failed to load commitment for slot %d", slot)
			}
			if err := stream.WriteBlob(writer, commitmentBytes); err != nil {
				return 0, ierrors.Wrapf(err, "failed to write commitment for slot %d", slot)
			}

			count++
		}

		return count, nil
	}); err != nil {
		return ierrors.Wrap(err, "failed to write commitments")
	}

	return nil
}

func (c *Commitments) Import(reader io.ReadSeeker) (err error) {
	if err := stream.ReadCollection(reader, func(i int) error {
		commitmentBytes, err := stream.ReadBlob(reader)
		if err != nil {
			return ierrors.Wrapf(err, "failed to read commitment at index %d", i)
		}

		commitment, err := model.CommitmentFromBytes(commitmentBytes, c.apiProvider)
		if err != nil {
			return ierrors.Wrapf(err, "failed to parse commitment at index %d", i)
		}

		if err := c.Store(commitment); err != nil {
			return ierrors.Wrapf(err, "failed to store commitment at index %d", i)
		}

		return nil
	}); err != nil {
		return ierrors.Wrap(err, "failed to read commitments")
	}

	return nil
}

func (c *Commitments) Rollback(targetSlot iotago.SlotIndex, lastCommittedSlot iotago.SlotIndex) error {
	for slot := targetSlot + 1; slot <= lastCommittedSlot; slot++ {
		if err := c.store.KVStore().Delete(lo.PanicOnErr(slot.Bytes())); err != nil {
			return ierrors.Wrapf(err, "failed to remove forked commitment for slot %d", slot)
		}
	}

	return nil
}
