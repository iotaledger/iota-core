package sybilprotection

import (
	"io"

	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection/seatmanager"
	iotago "github.com/iotaledger/iota.go/v4"
)

type SybilProtection interface {
	BlockAccepted(block *blocks.Block)
	ValidatorReward(validatorID iotago.AccountID, stakeAmount iotago.BaseToken, epochStart, epochEnd iotago.EpochIndex) (validatorReward iotago.Mana, err error)
	DelegatorReward(validatorID iotago.AccountID, delegatedAmount iotago.BaseToken, epochStart, epochEnd iotago.EpochIndex) (delegatorsReward iotago.Mana, err error)
	SeatManager() seatmanager.SeatManager
	CommitSlot(iotago.SlotIndex) (iotago.Identifier, iotago.Identifier)

	Import(io.ReadSeeker) error
	Export(io.WriteSeeker, iotago.SlotIndex) error

	module.Interface
}
