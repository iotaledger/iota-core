package blockgadget

import (
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
)

type Gadget interface {
	module.Module

	TrackWitnessWeight(votingBlock *blocks.Block)
	SetAccepted(block *blocks.Block) bool

	// Reset resets the component to a clean state as if it was created at the last commitment.
	Reset()
}
