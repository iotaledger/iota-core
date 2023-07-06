package account

import (
	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/serializer/v2/marshalutil"
	iotago "github.com/iotaledger/iota.go/v4"
)

const poolBytesLength = 3 * marshalutil.Uint64Size

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
	m := marshalutil.New(bytes)
	poolStake, err := m.ReadUint64()
	if err != nil {
		return nil, m.ReadOffset(), errors.Wrap(err, "failed to parse pool stake")
	}
	p.PoolStake = iotago.BaseToken(poolStake)

	validatorStake, err := m.ReadUint64()
	if err != nil {
		return nil, m.ReadOffset(), errors.Wrap(err, "failed to parse validator stake")
	}
	p.ValidatorStake = iotago.BaseToken(validatorStake)

	fixedCost, err := m.ReadUint64()
	if err != nil {
		return nil, m.ReadOffset(), errors.Wrap(err, "failed to parse fixed cost")
	}
	p.FixedCost = iotago.Mana(fixedCost)

	return p, m.ReadOffset(), nil
}

func (p *Pool) Bytes() (bytes []byte, err error) {
	m := marshalutil.New()
	m.WriteUint64(uint64(p.PoolStake))
	m.WriteUint64(uint64(p.ValidatorStake))
	m.WriteUint64(uint64(p.FixedCost))

	return m.Bytes(), nil
}
