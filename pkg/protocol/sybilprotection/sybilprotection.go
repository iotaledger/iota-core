package sybilprotection

import (
	"io"

	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection/seatmanager"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

type SybilProtection interface {
	TrackBlock(block *blocks.Block)
	EligibleValidators(epoch iotago.EpochIndex) (accounts.AccountsData, error)
	OrderedRegisteredCandidateValidatorsList(epoch iotago.EpochIndex) ([]*api.ValidatorResponse, error)
	IsCandidateActive(validatorID iotago.AccountID, epoch iotago.EpochIndex) (bool, error)
	// ValidatorReward returns the amount of mana that a validator with the given staking feature has earned in the feature's epoch range.
	//
	// The first epoch in which rewards existed is returned (firstRewardEpoch).
	// Since the validator may still be active and the EndEpoch might be in the future, the epoch until which rewards were calculated is returned in addition to the first epoch in which rewards existed (lastRewardEpoch).
	// The rewards are decayed until claimingEpoch, which should be set to the epoch in which the rewards would be claimed.
	ValidatorReward(validatorID iotago.AccountID, stakingFeature *iotago.StakingFeature, claimingEpoch iotago.EpochIndex) (validatorReward iotago.Mana, firstRewardEpoch iotago.EpochIndex, lastRewardEpoch iotago.EpochIndex, err error)
	// DelegatorReward returns the amount of mana that a delegator has earned in a given epoch range.
	//
	// The first epoch in which rewards existed is returned (firstRewardEpoch).
	// Since the Delegation Output's EndEpoch might be unset due to an ongoing delegation, the epoch until which rewards were calculated is also returned (lastRewardEpoch).
	// The rewards are decayed until claimingEpoch, which should be set to the epoch in which the rewards would be claimed.
	DelegatorReward(validatorID iotago.AccountID, delegatedAmount iotago.BaseToken, epochStart iotago.EpochIndex, epochEnd iotago.EpochIndex, claimingEpoch iotago.EpochIndex) (delegatorReward iotago.Mana, firstRewardEpoch iotago.EpochIndex, lastRewardEpoch iotago.EpochIndex, err error)
	SeatManager() seatmanager.SeatManager
	CommitSlot(iotago.SlotIndex) (iotago.Identifier, iotago.Identifier, error)
	Import(io.ReadSeeker) error
	Export(io.WriteSeeker, iotago.SlotIndex) error

	// Reset resets the component to a clean state as if it was created at the last commitment.
	Reset()

	module.Interface
}
