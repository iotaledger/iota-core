package sybilprotection

import (
	"io"

	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection/seatmanager"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/nodeclient/apimodels"
)

type SybilProtection interface {
	TrackValidationBlock(block *blocks.Block)
	EligibleValidators(epoch iotago.EpochIndex) (accounts.AccountsData, error)
	OrderedRegisteredCandidateValidatorsList(epoch iotago.EpochIndex) ([]*apimodels.ValidatorResponse, error)
	IsCandidateActive(validatorID iotago.AccountID, epoch iotago.EpochIndex) bool
	// ValidatorReward returns the amount of mana that a validator has earned in a given epoch range.
	// The actual used epoch range is returned, only until usedEnd the decay was applied.
	ValidatorReward(validatorID iotago.AccountID, stakeAmount iotago.BaseToken, epochStart, epochEnd iotago.EpochIndex) (validatorReward iotago.Mana, decayedStart, decayedEnd iotago.EpochIndex, err error)
	// DelegatorReward returns the amount of mana that a delegator has earned in a given epoch range.
	// The actual used epoch range is returned, only until usedEnd the decay was applied.
	DelegatorReward(validatorID iotago.AccountID, delegatedAmount iotago.BaseToken, epochStart, epochEnd iotago.EpochIndex) (delegatorsReward iotago.Mana, decayedStart, decayedEnd iotago.EpochIndex, err error)
	SeatManager() seatmanager.SeatManager
	CommitSlot(iotago.SlotIndex) (iotago.Identifier, iotago.Identifier, error)
	Import(io.ReadSeeker) error
	Export(io.WriteSeeker, iotago.SlotIndex) error

	module.Interface
}
