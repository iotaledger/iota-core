package performance

import (
	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/serializer/v2/marshalutil"
	iotago "github.com/iotaledger/iota.go/v4"
)

<<<<<<< HEAD:pkg/protocol/engine/rewards/types.go
// Pool represents all the data we need for a given validator and epoch to calculate its rewards data.
type Pool struct {
	// Total stake of the pool, including delegators
	PoolStake iotago.BaseToken
	// Validator's stake
	ValidatorStake iotago.BaseToken
	FixedCost      iotago.Mana
=======
type RewardsForAccount struct {
	// Total stake of the validator including delegations
	PoolStake uint64
	// Rewards normalized by performance factor
	PoolRewards uint64
	// What the validator charges for its staking duties
	FixedCost uint64
}

func (r RewardsForAccount) Bytes() (bytes []byte, err error) {
	m := marshalutil.New()
	m.WriteUint64(r.PoolStake)
	m.WriteUint64(r.PoolRewards)
	m.WriteUint64(r.FixedCost)

	return m.Bytes(), nil
}

func (r *RewardsForAccount) FromBytes(bytes []byte) (n int, err error) {
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
>>>>>>> 0e8154af (Epoch orchestrator, rewards and performance tracker as epochgadget):pkg/protocol/engine/consensus/epochgadget/epochorchestrator/performance/types.go
}

// PoolsStats contains stats about the pools from all the validators for an epoch.
type PoolsStats struct {
	TotalStake          iotago.BaseToken
	TotalValidatorStake iotago.BaseToken
	ProfitMargin        uint64
}

func (p *PoolsStats) FromBytes(bytes []byte) (n int, err error) {
	m := marshalutil.New(bytes)
	totalStake, err := m.ReadUint64()
	if err != nil {
		return m.ReadOffset(), errors.Wrap(err, "failed to parse total stake")
	}
	p.TotalStake = iotago.BaseToken(totalStake)

	totalValidatorStake, err := m.ReadUint64()
	if err != nil {
		return m.ReadOffset(), errors.Wrap(err, "failed to parse total validator stake")
	}
	p.TotalValidatorStake = iotago.BaseToken(totalValidatorStake)

	p.ProfitMargin, err = m.ReadUint64()
	if err != nil {
		return m.ReadOffset(), errors.Wrap(err, "failed to parse profit margin")
	}

	return m.ReadOffset(), nil
}

func (p PoolsStats) Bytes() (bytes []byte, err error) {
	m := marshalutil.New()
	m.WriteUint64(uint64(p.TotalStake))
	m.WriteUint64(uint64(p.TotalValidatorStake))
	m.WriteUint64(p.ProfitMargin)

	return m.Bytes(), nil
}
<<<<<<< HEAD:pkg/protocol/engine/rewards/types.go

type AccountRewards struct {
	// Total stake of the validator including delegations
	PoolStake iotago.BaseToken
	// Rewards normalized by performance factor
	PoolRewards iotago.Mana
	// What the validator charges for its staking duties
	FixedCost iotago.Mana
}

func (r *AccountRewards) FromBytes(bytes []byte) (n int, err error) {
	m := marshalutil.New(bytes)

	poolStake, err := m.ReadUint64()
	if err != nil {
		return m.ReadOffset(), errors.Wrap(err, "failed to parse pool stake")
	}
	r.PoolStake = iotago.BaseToken(poolStake)

	poolRewards, err := m.ReadUint64()
	if err != nil {
		return m.ReadOffset(), errors.Wrap(err, "failed to parse pool rewards")
	}
	r.PoolRewards = iotago.Mana(poolRewards)

	fixedCost, err := m.ReadUint64()
	if err != nil {
		return m.ReadOffset(), errors.Wrap(err, "failed to parse fixed cost")
	}
	r.FixedCost = iotago.Mana(fixedCost)

	return m.ReadOffset(), nil
}

func (r AccountRewards) Bytes() (bytes []byte, err error) {
	m := marshalutil.New()
	m.WriteUint64(uint64(r.PoolStake))
	m.WriteUint64(uint64(r.PoolRewards))
	m.WriteUint64(uint64(r.FixedCost))

	return m.Bytes(), nil
}
=======
>>>>>>> 0e8154af (Epoch orchestrator, rewards and performance tracker as epochgadget):pkg/protocol/engine/consensus/epochgadget/epochorchestrator/performance/types.go
