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

	// GetOrRequestBlock returns the Block with the given BlockID from the BlockDAG (and requests it from the network if
	// it is missing). If the requested Block is below the eviction threshold, then this method will return a nil block
	// without requesting it.
	GetOrRequestBlock(blockID iotago.BlockID) (block *blocks.Block, requested bool)

	// Reset resets the BlockDAG to its initial state after the last commitment.
	Reset()

	module.Interface
}
