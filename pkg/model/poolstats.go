package model

import (
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/serializer/v2/marshalutil"
	iotago "github.com/iotaledger/iota.go/v4"
)

// PoolsStats contains stats about the pools from all the validators for an epoch.
type PoolsStats struct {
	TotalStake          iotago.BaseToken
	TotalValidatorStake iotago.BaseToken
	ProfitMargin        uint64
}

func PoolsStatsFromBytes(bytes []byte) (*PoolsStats, int, error) {
	p := new(PoolsStats)
	m := marshalutil.New(bytes)
	totalStake, err := m.ReadUint64()
	if err != nil {
		return nil, m.ReadOffset(), ierrors.Wrap(err, "failed to parse total stake")
	}
	p.TotalStake = iotago.BaseToken(totalStake)

	totalValidatorStake, err := m.ReadUint64()
	if err != nil {
		return nil, m.ReadOffset(), ierrors.Wrap(err, "failed to parse total validator stake")
	}
	p.TotalValidatorStake = iotago.BaseToken(totalValidatorStake)

	p.ProfitMargin, err = m.ReadUint64()
	if err != nil {
		return nil, m.ReadOffset(), ierrors.Wrap(err, "failed to parse profit margin")
	}

	return p, m.ReadOffset(), nil
}

func (p *PoolsStats) Bytes() ([]byte, error) {
	m := marshalutil.New()
	m.WriteUint64(uint64(p.TotalStake))
	m.WriteUint64(uint64(p.TotalValidatorStake))
	m.WriteUint64(p.ProfitMargin)

	return m.Bytes(), nil
}

type PoolRewards struct {
	// Total stake of the validator including delegations
	PoolStake iotago.BaseToken
	// Rewards normalized by performance factor
	PoolRewards iotago.Mana
	// What the validator charges for its staking duties
	FixedCost iotago.Mana
}

func PoolRewardsFromBytes(bytes []byte) (*PoolRewards, int, error) {
	p := new(PoolRewards)
	m := marshalutil.New(bytes)

	poolStake, err := m.ReadUint64()
	if err != nil {
		return nil, m.ReadOffset(), ierrors.Wrap(err, "failed to parse pool stake")
	}
	p.PoolStake = iotago.BaseToken(poolStake)

	poolRewards, err := m.ReadUint64()
	if err != nil {
		return nil, m.ReadOffset(), ierrors.Wrap(err, "failed to parse pool rewards")
	}
	p.PoolRewards = iotago.Mana(poolRewards)

	fixedCost, err := m.ReadUint64()
	if err != nil {
		return nil, m.ReadOffset(), ierrors.Wrap(err, "failed to parse fixed cost")
	}
	p.FixedCost = iotago.Mana(fixedCost)

	return p, m.ReadOffset(), nil
}

func (p *PoolRewards) Bytes() ([]byte, error) {
	m := marshalutil.New()
	m.WriteUint64(uint64(p.PoolStake))
	m.WriteUint64(uint64(p.PoolRewards))
	m.WriteUint64(uint64(p.FixedCost))

	return m.Bytes(), nil
}
