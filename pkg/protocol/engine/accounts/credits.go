package accounts

import (
	"context"

	"github.com/iotaledger/hive.go/serializer/v2/serix"
	iotago "github.com/iotaledger/iota.go/v4"
)

// Credits is a weight annotated with the slot it was last updated in.
type Credits struct {
	Value      int64            `serix:"0"`
	UpdateTime iotago.SlotIndex `serix:"1"`
}

// NewCredits creates a new Credits instance.
func NewCredits(value int64, updateTime iotago.SlotIndex) (newCredits *Credits) {
	return &Credits{
		Value:      value,
		UpdateTime: updateTime,
	}
}

// Bytes returns a serialized version of the Credits.
func (c Credits) Bytes() ([]byte, error) {
	return serix.DefaultAPI.Encode(context.Background(), c)
}

// FromBytes parses a serialized version of the Credits.
func (c *Credits) FromBytes(bytes []byte) (int, error) {
	return serix.DefaultAPI.Decode(context.Background(), bytes, c)
}

// Update updates the Credits increasing Value and updateTime.
func (c *Credits) Update(add int64, updateTime ...iotago.SlotIndex) {
	c.Value += add
	if len(updateTime) > 0 {
		c.UpdateTime = updateTime[0]
	}
}
