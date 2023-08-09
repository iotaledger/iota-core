package blockdag

import (
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	iotago "github.com/iotaledger/iota.go/v4"
)

type BlockDAG interface {
	// Attach is used to attach new Blocks to the BlockDAG. It is the main function of the BlockDAG that triggers Events.
	Attach(data *model.Block) (block *blocks.Block, wasAttached bool, err error)

	// GetOrRequestBlock returns the Block with the given BlockID if it is known to the BlockDAG. If it is not known, it
	// will be requested.
	GetOrRequestBlock(blockID iotago.BlockID) (block *blocks.Block, requested bool)

	// SetInvalid marks a Block as invalid.
	SetInvalid(block *blocks.Block, reason error) (wasUpdated bool)

	module.Interface
}
