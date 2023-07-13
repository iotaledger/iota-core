package account

import (
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/serializer/v2/marshalutil"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Validators map[iotago.AccountID]*Validator

// Validator represents all the essential data describing a validator.
type Validator struct {
	// Total stake of the pool, including delegators
	PoolStake iotago.BaseToken
	// Validator's stake
	ValidatorStake                 iotago.BaseToken
	FixedCost                      iotago.Mana
	StakingEnd                     iotago.EpochIndex
	LatestSupportedProtocolVersion uint8
}

func ValidatorFromBytes(bytes []byte) (*Validator, int, error) {
	p := new(Validator)
	m := marshalutil.New(bytes)
	poolStake, err := m.ReadUint64()
	if err != nil {
		return nil, m.ReadOffset(), ierrors.Wrap(err, "failed to parse pool stake")
	}
	p.PoolStake = iotago.BaseToken(poolStake)

	validatorStake, err := m.ReadUint64()
	if err != nil {
		return nil, m.ReadOffset(), ierrors.Wrap(err, "failed to parse validator stake")
	}
	p.ValidatorStake = iotago.BaseToken(validatorStake)

	fixedCost, err := m.ReadUint64()
	if err != nil {
		return nil, m.ReadOffset(), ierrors.Wrap(err, "failed to parse fixed cost")
	}
	p.FixedCost = iotago.Mana(fixedCost)

	stakingEndEpoch, err := m.ReadUint64()
	if err != nil {
		return nil, m.ReadOffset(), ierrors.Wrap(err, "failed to parse staking end epoch")
	}
	p.StakingEnd = iotago.EpochIndex(stakingEndEpoch)

	return p, m.ReadOffset(), nil
}

func (p *Validator) Bytes() (bytes []byte, err error) {
	m := marshalutil.New()
	m.WriteUint64(uint64(p.PoolStake))
	m.WriteUint64(uint64(p.ValidatorStake))
	m.WriteUint64(uint64(p.FixedCost))
	m.WriteUint64(uint64(p.StakingEnd))

	return m.Bytes(), nil
}
