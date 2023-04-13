package blockdag

import (
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/model"
	iotago "github.com/iotaledger/iota.go/v4"
)

type BlockDAG interface {
	// Attach is used to attach new Blocks to the BlockDAG. It is the main function of the BlockDAG that triggers Events.
	Attach(data *model.Block) (block *Block, wasAttached bool, err error)

	// Block retrieves a Block with metadata from the in-memory storage of the BlockDAG.
	Block(id iotago.BlockID) (block *Block, exists bool)

	// SetOrphaned marks a Block as orphaned and propagates it to its future cone.
	SetOrphaned(block *Block) (updated bool)

	// SetInvalid marks a Block as invalid and propagates the invalidity to its future cone.
	SetInvalid(block *Block, reason error) (wasUpdated bool)

	module.Interface
}
