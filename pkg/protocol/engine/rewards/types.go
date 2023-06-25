package rewards

import (
	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/serializer/v2/marshalutil"
)

// Pool represents all the data we need for a given validator and epoch to calculate its rewards data.
type Pool struct {
	// Total stake of the pool, including delegators
	PoolStake uint64
	// Validator's stake
	ValidatorStake uint64
	FixedCost      uint64
}

// PoolsStats contains stats about the pools from all the validators for an epoch.
type PoolsStats struct {
	TotalStake          uint64
	TotalValidatorStake uint64
	ProfitMargin        uint64
}

func (p *PoolsStats) FromBytes(bytes []byte) (n int, err error) {
	m := marshalutil.New(bytes)
	p.TotalStake, err = m.ReadUint64()
	if err != nil {
		return m.ReadOffset(), errors.Wrap(err, "failed to parse total stake")
	}

	p.TotalValidatorStake, err = m.ReadUint64()
	if err != nil {
		return m.ReadOffset(), errors.Wrap(err, "failed to parse total validator stake")
	}

	p.ProfitMargin, err = m.ReadUint64()
	if err != nil {
		return m.ReadOffset(), errors.Wrap(err, "failed to parse profit margin")
	}

	return m.ReadOffset(), nil
}

func (p PoolsStats) Bytes() (bytes []byte, err error) {
	m := marshalutil.New()
	m.WriteUint64(p.TotalStake)
	m.WriteUint64(p.TotalValidatorStake)
	m.WriteUint64(p.ProfitMargin)

	return m.Bytes(), nil
}

type AccountRewards struct {
	// Total stake of the validator including delegations
	PoolStake uint64
	// Rewards normalized by performance factor
	PoolRewards uint64
	// What the validator charges for its staking duties
	FixedCost uint64
}

func (r *AccountRewards) FromBytes(bytes []byte) (n int, err error) {
	m := marshalutil.New(bytes)

	r.PoolStake, err = m.ReadUint64()
	if err != nil {
		return m.ReadOffset(), errors.Wrap(err, "failed to parse pool stake")
	}

	r.PoolRewards, err = m.ReadUint64()
	if err != nil {
		return m.ReadOffset(), errors.Wrap(err, "failed to parse pool rewards")
	}

	r.FixedCost, err = m.ReadUint64()
	if err != nil {
		return m.ReadOffset(), errors.Wrap(err, "failed to parse fixed cost")
	}

	return m.ReadOffset(), nil
}

func (r AccountRewards) Bytes() (bytes []byte, err error) {
	m := marshalutil.New()
	m.WriteUint64(r.PoolStake)
	m.WriteUint64(r.PoolRewards)
	m.WriteUint64(r.FixedCost)

	return m.Bytes(), nil
}
