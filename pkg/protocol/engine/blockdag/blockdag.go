package blockdag

import (
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	iotago "github.com/iotaledger/iota.go/v4"
)

type BlockDAG interface {
	// Append is used to append new Blocks to the BlockDAG. It is the main function of the BlockDAG that triggers Events.
	Append(modelBlock *model.Block) (block *blocks.Block, wasAppended bool, err error)

	// GetOrRequestBlock returns the Block with the given BlockID from the BlockDAG (and requests it from the network if
	// it is missing). If the requested Block is below the eviction threshold, then this method will return a nil block
	// without requesting it.
	GetOrRequestBlock(blockID iotago.BlockID) (block *blocks.Block, requested bool)

	// Reset resets the component to a clean state as if it was created at the last commitment.
	Reset()

	module.Module
}
