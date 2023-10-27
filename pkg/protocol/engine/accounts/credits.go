package accounts

import (
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/serializer/v2"
	"github.com/iotaledger/hive.go/serializer/v2/stream"
	iotago "github.com/iotaledger/iota.go/v4"
)

const BlockIssuanceCreditsBytesLength = serializer.Int64ByteSize + serializer.UInt32ByteSize

// BlockIssuanceCredits is a weight annotated with the slot it was last updated in.
type BlockIssuanceCredits struct {
	Value      iotago.BlockIssuanceCredits
	UpdateSlot iotago.SlotIndex
}

// NewBlockIssuanceCredits creates a new Credits instance.
func NewBlockIssuanceCredits(value iotago.BlockIssuanceCredits, updateTime iotago.SlotIndex) (newCredits *BlockIssuanceCredits) {
	return &BlockIssuanceCredits{
		Value:      value,
		UpdateSlot: updateTime,
	}
}

// Bytes returns a serialized version of the Credits.
func (c *BlockIssuanceCredits) Bytes() ([]byte, error) {
	byteBuffer := stream.NewByteBuffer()

	if err := stream.Write(byteBuffer, c.Value); err != nil {
		return nil, ierrors.Wrap(err, "failed to write value")
	}

	if err := stream.Write(byteBuffer, c.UpdateSlot); err != nil {
		return nil, ierrors.Wrap(err, "failed to write updateTime")
	}

	return byteBuffer.Bytes()
}

func BlockIssuanceCreditsFromBytes(bytes []byte) (*BlockIssuanceCredits, int, error) {
	c := new(BlockIssuanceCredits)

	var err error
	byteReader := stream.NewByteReader(bytes)

	if c.Value, err = stream.Read[iotago.BlockIssuanceCredits](byteReader); err != nil {
		return nil, 0, ierrors.Wrap(err, "failed to read value")
	}

	if c.UpdateSlot, err = stream.Read[iotago.SlotIndex](byteReader); err != nil {
		return nil, 0, ierrors.Wrap(err, "failed to read updateTime")
	}

	return c, byteReader.BytesRead(), nil
}

// Update updates the Credits increasing Value and updateTime.
func (c *BlockIssuanceCredits) Update(change iotago.BlockIssuanceCredits, updateSlot ...iotago.SlotIndex) {
	c.Value += change
	if len(updateSlot) > 0 {
		c.UpdateSlot = updateSlot[0]
	}
}
