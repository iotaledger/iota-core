package model

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/serializer/v2/serix"
	"github.com/iotaledger/hive.go/stringify"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Commitment struct {
	api iotago.API

	commitmentID iotago.CommitmentID

	data           []byte
	commitmentOnce sync.Once
	commitment     *iotago.Commitment
}

func NewEmptyCommitment(api iotago.API) *Commitment {
	return lo.PanicOnErr(CommitmentFromCommitment(iotago.NewEmptyCommitment(), api))
}

func CommitmentFromCommitment(iotaCommitment *iotago.Commitment, api iotago.API, opts ...serix.Option) (*Commitment, error) {
	data, err := api.Encode(iotaCommitment, opts...)
	if err != nil {
		return nil, err
	}

	commitmentID, err := iotaCommitment.ID()
	if err != nil {
		return nil, err
	}

	commitment := &Commitment{
		api:          api,
		commitmentID: commitmentID,
		data:         data,
	}

	commitment.commitmentOnce.Do(func() {
		commitment.commitment = iotaCommitment
	})

	return commitment, nil
}

func CommitmentFromIDAndBytes(commitmentID iotago.CommitmentID, data []byte, api iotago.API, opts ...serix.Option) (*Commitment, error) {
	iotaCommitment := new(iotago.Commitment)
	if _, err := api.Decode(data, iotaCommitment, opts...); err != nil {
		return nil, err
	}

	commitment := &Commitment{
		api:          api,
		commitmentID: commitmentID,
		data:         data,
	}

	commitment.commitmentOnce.Do(func() {
		commitment.commitment = iotaCommitment
	})

	return commitment, nil
}

func CommitmentFromBytes(data []byte, api iotago.API, opts ...serix.Option) (*Commitment, error) {
	iotaCommitment := new(iotago.Commitment)
	if _, err := api.Decode(data, iotaCommitment, opts...); err != nil {
		return nil, err
	}

	commitmentID, err := iotaCommitment.ID()
	if err != nil {
		return nil, err
	}

	return CommitmentFromIDAndBytes(commitmentID, data, api, opts...)
}

func (c *Commitment) ID() iotago.CommitmentID {
	return c.commitmentID
}

func (c *Commitment) Index() iotago.SlotIndex {
	return c.Commitment().Index
}

func (c *Commitment) PrevID() iotago.CommitmentID {
	return c.Commitment().PrevID
}

func (c *Commitment) RootsID() iotago.Identifier {
	return c.Commitment().RootsID
}

func (c *Commitment) CumulativeWeight() int64 {
	return c.Commitment().CumulativeWeight
}

func (c *Commitment) Data() []byte {
	return c.data
}

func (c *Commitment) Commitment() *iotago.Commitment {
	c.commitmentOnce.Do(func() {
		iotaCommitment := new(iotago.Commitment)
		// No need to verify the commitment again here
		if _, err := c.api.Decode(c.data, iotaCommitment); err != nil {
			panic(fmt.Sprintf("failed to deserialize commitment: %v, error: %s", c.commitmentID.ToHex(), err))
		}

		c.commitment = iotaCommitment
	})

	return c.commitment
}

func (c *Commitment) String() string {
	encode, err := c.api.JSONEncode(c.Commitment())
	if err != nil {
		panic(err)
	}
	var out bytes.Buffer
	if json.Indent(&out, encode, "", "  ") != nil {
		panic(err)
	}

	return stringify.Struct("Commitment",
		stringify.NewStructField("ID", c.ID()),
		stringify.NewStructField("Commitment", out.String()),
	)
}
