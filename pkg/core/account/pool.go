package account

import (
	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/serializer/v2/marshalutil"
)

const poolBytesLength = 3 * marshalutil.Uint64Size

// Pool represents all the data we need for a given validator and epoch to calculate its rewards data.
type Pool struct {
	// Total stake of the pool, including delegators
	PoolStake uint64
	// Validator's stake
	ValidatorStake uint64
	FixedCost      uint64
}

func (p *Pool) FromBytes(bytes []byte) (n int, err error) {
	m := marshalutil.New(bytes)
	p.PoolStake, err = m.ReadUint64()
	if err != nil {
		return m.ReadOffset(), errors.Wrap(err, "failed to parse pool stake")
	}

	p.ValidatorStake, err = m.ReadUint64()
	if err != nil {
		return m.ReadOffset(), errors.Wrap(err, "failed to parse validator stake")
	}

	p.FixedCost, err = m.ReadUint64()
	if err != nil {
		return m.ReadOffset(), errors.Wrap(err, "failed to parse fixed cost")
	}

	return m.ReadOffset(), nil
}

func (p Pool) Bytes() (bytes []byte, err error) {
	m := marshalutil.New()
	m.WriteUint64(p.PoolStake)
	m.WriteUint64(p.ValidatorStake)
	m.WriteUint64(p.FixedCost)

	return m.Bytes(), nil
}
