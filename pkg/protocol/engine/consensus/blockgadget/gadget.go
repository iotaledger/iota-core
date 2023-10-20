package blockgadget

import (
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
)

type Gadget interface {
	module.Interface

	TrackWitnessWeight(votingBlock *blocks.Block)
	SetAccepted(block *blocks.Block) bool

	Reset()
}
