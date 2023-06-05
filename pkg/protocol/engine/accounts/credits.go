package accounts

import (
	"context"

	"github.com/iotaledger/hive.go/serializer/v2/serix"
	iotago "github.com/iotaledger/iota.go/v4"
)

// BlockIssuanceCredits is a weight annotated with the slot it was last updated in.
type BlockIssuanceCredits struct {
	Value      int64            `serix:"0"`
	UpdateTime iotago.SlotIndex `serix:"1"`
}

// NewBlockIssuanceCredits creates a new Credits instance.
func NewBlockIssuanceCredits(value int64, updateTime iotago.SlotIndex) (newCredits *BlockIssuanceCredits) {
	return &BlockIssuanceCredits{
		Value:      value,
		UpdateTime: updateTime,
	}
}

// Bytes returns a serialized version of the Credits.
func (c BlockIssuanceCredits) Bytes() ([]byte, error) {
	return serix.DefaultAPI.Encode(context.Background(), c)
}

// FromBytes parses a serialized version of the Credits.
func (c *BlockIssuanceCredits) FromBytes(bytes []byte) (int, error) {
	return serix.DefaultAPI.Decode(context.Background(), bytes, c)
}

// Update updates the Credits increasing Value and updateTime.
func (c *BlockIssuanceCredits) Update(change int64, updateTime ...iotago.SlotIndex) {
	c.Value += change
	if len(updateTime) > 0 {
		c.UpdateTime = updateTime[0]
	}
}
