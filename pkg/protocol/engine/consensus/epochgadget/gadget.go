package epochgadget

import (
	"io"

	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Gadget interface {
	BlockAccepted(block *blocks.Block)
	ValidatorReward(validatorID iotago.AccountID, stakeAmount uint64, epochStart, epochEnd iotago.EpochIndex) (validatorReward uint64, err error)
	DelegatorReward(validatorID iotago.AccountID, delegatedAmount uint64, epochStart, epochEnd iotago.EpochIndex) (delegatorsReward uint64, err error)

	Import(io.ReadSeeker) error
	Export(io.WriteSeeker, iotago.SlotIndex) error

	module.Interface
}
