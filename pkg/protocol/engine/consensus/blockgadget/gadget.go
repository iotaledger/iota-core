package blockgadget

import (
	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Gadget interface {
	module.Interface

	LastAcceptedBlockIndex() iotago.SlotIndex

	LastAcceptedBlockIndexR() reactive.Variable[iotago.SlotIndex]

	LastConfirmedBlockIndex() iotago.SlotIndex

	LastConfirmedBlockIndexR() reactive.Variable[iotago.SlotIndex]

	TrackWitnessWeight(votingBlock *blocks.Block)
}
