package account

import (
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/serializer/v2"
	"github.com/iotaledger/hive.go/serializer/v2/stream"
	iotago "github.com/iotaledger/iota.go/v4"
)

const poolBytesLength = 3 * serializer.UInt64ByteSize

// Pool represents all the data we need for a given validator and epoch to calculate its rewards data.
type Pool struct {
	// Total stake of the pool, including delegators
	PoolStake iotago.BaseToken
	// Validator's stake
	ValidatorStake iotago.BaseToken
	FixedCost      iotago.Mana
}

func PoolFromBytes(bytes []byte) (*Pool, int, error) {
	p := new(Pool)

	var err error
	byteReader := stream.NewByteReader(bytes)

	if p.PoolStake, err = stream.Read[iotago.BaseToken](byteReader); err != nil {
		return nil, 0, ierrors.Wrap(err, "failed to read PoolStake")
	}
	if p.ValidatorStake, err = stream.Read[iotago.BaseToken](byteReader); err != nil {
		return nil, 0, ierrors.Wrap(err, "failed to read ValidatorStake")
	}
	if p.FixedCost, err = stream.Read[iotago.Mana](byteReader); err != nil {
		return nil, 0, ierrors.Wrap(err, "failed to read FixedCost")
	}

	return p, byteReader.BytesRead(), nil
}

func (p *Pool) Bytes() ([]byte, error) {
	byteBuffer := stream.NewByteBuffer(poolBytesLength)

	if err := stream.Write(byteBuffer, p.PoolStake); err != nil {
		return nil, ierrors.Wrap(err, "failed to write PoolStake")
	}
	if err := stream.Write(byteBuffer, p.ValidatorStake); err != nil {
		return nil, ierrors.Wrap(err, "failed to write ValidatorStake")
	}
	if err := stream.Write(byteBuffer, p.FixedCost); err != nil {
		return nil, ierrors.Wrap(err, "failed to write FixedCost")
	}

	return byteBuffer.Bytes()
}
