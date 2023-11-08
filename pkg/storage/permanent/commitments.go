package permanent

import (
	"io"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/serializer/v2"
	"github.com/iotaledger/hive.go/serializer/v2/stream"
	"github.com/iotaledger/iota-core/pkg/model"
	iotago "github.com/iotaledger/iota.go/v4"
)

var (
	ErrCommitmentBeforeGenesis = ierrors.New("commitment is before genesis")
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
			(*model.Commitment).Bytes,
			model.CommitmentFromBytes(apiProvider),
		),
	}
}

func (c *Commitments) Store(commitment *model.Commitment) error {
	return c.store.Set(commitment.Commitment().Slot, commitment)
}

func (c *Commitments) Load(slot iotago.SlotIndex) (commitment *model.Commitment, err error) {
	genesisSlot := c.apiProvider.CommittedAPI().ProtocolParameters().GenesisSlot()
	if slot < genesisSlot {
		return nil, ierrors.Wrapf(ErrCommitmentBeforeGenesis, "slot %d is before genesis slot %d", slot, genesisSlot)
	}

	return c.store.Get(slot)
}

func (c *Commitments) Export(writer io.WriteSeeker, targetSlot iotago.SlotIndex) (err error) {
	if err := stream.WriteCollection(writer, serializer.SeriLengthPrefixTypeAsUint32, func() (elementsCount int, err error) {
		var count int
		for slot := c.apiProvider.CommittedAPI().ProtocolParameters().GenesisSlot(); slot <= targetSlot; slot++ {
			commitmentBytes, err := c.store.KVStore().Get(lo.PanicOnErr(slot.Bytes()))
			if err != nil {
				return 0, ierrors.Wrapf(err, "failed to load commitment for slot %d", slot)
			}

			if err := stream.WriteBytesWithSize(writer, commitmentBytes, serializer.SeriLengthPrefixTypeAsUint16); err != nil {
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
	if err := stream.ReadCollection(reader, serializer.SeriLengthPrefixTypeAsUint32, func(i int) error {
		commitment, err := stream.ReadObjectWithSize[*model.Commitment](reader, serializer.SeriLengthPrefixTypeAsUint16, model.CommitmentFromBytes(c.apiProvider))
		if err != nil {
			return ierrors.Wrapf(err, "failed to read commitment at index %d", i)
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
