package blockdag

import (
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
)

type BlockDAG interface {
	// Attach is used to attach new Blocks to the BlockDAG. It is the main function of the BlockDAG that triggers Events.
	Attach(data *model.Block) (block *blocks.Block, wasAttached bool, err error)

	// SetOrphaned marks a Block as orphaned and propagates it to its future cone.
	SetOrphaned(block *blocks.Block) (updated bool)

	// SetInvalid marks a Block as invalid.
	SetInvalid(block *blocks.Block, reason error) (wasUpdated bool)

	module.Interface
}
