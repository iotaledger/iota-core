package model

import (
	"bytes"
	"encoding/json"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/serializer/v2/serix"
	"github.com/iotaledger/hive.go/stringify"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Commitment struct {
	api iotago.API

	commitmentID iotago.CommitmentID

	data       []byte
	commitment *iotago.Commitment
}

func NewEmptyCommitment(api iotago.API) *Commitment {
	emptyCommitment := iotago.NewEmptyCommitment(api)

	return lo.PanicOnErr(CommitmentFromCommitment(emptyCommitment, api))
}

func newCommitment(commitmentID iotago.CommitmentID, iotaCommitment *iotago.Commitment, data []byte, api iotago.API) (*Commitment, error) {
	return &Commitment{
		api:          api,
		commitmentID: commitmentID,
		data:         data,
		commitment:   iotaCommitment,
	}, nil
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

	return newCommitment(commitmentID, iotaCommitment, data, api)
}

func CommitmentFromBytes(apiProvider iotago.APIProvider) func([]byte) (*Commitment, int, error) {
	return func(bytes []byte) (*Commitment, int, error) {
		var totalBytesRead int

		// We read the version byte here to determine the API to use, but then we decode the entire commitment again.
		// Thus, we don't count the version byte as read bytes.
		version, _, err := iotago.VersionFromBytes(bytes)
		if err != nil {
			return nil, 0, ierrors.Wrap(err, "failed to determine version")
		}

		apiForVersion, err := apiProvider.APIForVersion(version)
		if err != nil {
			return nil, 0, ierrors.Wrapf(err, "failed to get API for version %d", version)
		}

		iotaCommitment := new(iotago.Commitment)
		if totalBytesRead, err = apiForVersion.Decode(bytes, iotaCommitment, serix.WithValidation()); err != nil {
			return nil, 0, ierrors.Wrap(err, "failed to decode commitment")
		}

		commitmentID, err := iotaCommitment.ID()
		if err != nil {
			return nil, 0, ierrors.Wrap(err, "failed to determine commitment ID")
		}

		commitment, err := newCommitment(commitmentID, iotaCommitment, bytes, apiForVersion)
		if err != nil {
			return nil, 0, ierrors.Wrap(err, "failed to create commitment")
		}

		return commitment, totalBytesRead, nil
	}
}

func (c *Commitment) ID() iotago.CommitmentID {
	return c.commitmentID
}

func (c *Commitment) Slot() iotago.SlotIndex {
	return c.Commitment().Slot
}

func (c *Commitment) PreviousCommitmentID() iotago.CommitmentID {
	return c.Commitment().PreviousCommitmentID
}

func (c *Commitment) RootsID() iotago.Identifier {
	return c.Commitment().RootsID
}

func (c *Commitment) CumulativeWeight() uint64 {
	return c.Commitment().CumulativeWeight
}

func (c *Commitment) ReferenceManaCost() iotago.Mana {
	return c.Commitment().ReferenceManaCost
}

func (c *Commitment) Data() []byte {
	return c.data
}

func (c *Commitment) Bytes() ([]byte, error) {
	return c.data, nil
}

func (c *Commitment) Commitment() *iotago.Commitment {
	return c.commitment
}

func (c *Commitment) String() string {
	encode, err := c.api.JSONEncode(c.Commitment())
	if err != nil {
		panic(err)
	}
	var out bytes.Buffer
	if err = json.Indent(&out, encode, "", "  "); err != nil {
		panic(err)
	}

	return stringify.Struct("Commitment",
		stringify.NewStructField("ID", c.ID()),
		stringify.NewStructField("Commitment", out.String()),
	)
}
