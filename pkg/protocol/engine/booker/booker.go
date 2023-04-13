package booker

import (
	"github.com/iotaledger/hive.go/runtime/module"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Booker interface {
	// Block retrieves a Block with metadata from the in-memory storage of the Booker.
	Block(id iotago.BlockID) (block *Block, exists bool)

	module.Interface
}
