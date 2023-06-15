package tipmanager

import (
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
)

type TipSelection interface {
	AddBlock(block *blocks.Block) TipMetadata

	// SelectTips selects the tips that should be used for the next block.
	SelectTips(count int) (references model.ParentReferences)

	TipManager() TipManager

	// Events returns the events of the TipManager.
	Events() *Events

	// Interface embeds the required methods of the module.Interface.
	module.Interface
}
