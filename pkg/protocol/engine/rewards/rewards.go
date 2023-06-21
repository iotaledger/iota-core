package rewards

import iotago "github.com/iotaledger/iota.go/v4"

type Rewards interface {
	DelegatorReward(validatorID iotago.AccountID, delegatedAmount int64, epochStart, epochEnd iotago.EpochIndex) (delegatorReward uint64, err error)
	ValidatorReward(validatorID iotago.AccountID, stakeAmount int64, epochStart, epochEnd iotago.EpochIndex, fixedCost uint64) (validatorReward uint64, err error)
	// this is not mandatory
	PoolReward(validatorID iotago.AccountID, epochIndex iotago.SlotIndex) (poolStake, poolRewards uint64, err error)
	// todo define it
	CommitRewards() (root iotago.Identifier, err error)
}
