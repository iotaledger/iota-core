package tipmanager

import (
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	iotago "github.com/iotaledger/iota.go/v4"
)

// TipManager is a component that maintains a perception of the unreferenced Blocks of the Tangle.
type TipManager interface {
	// AddBlock adds a block to the TipManager.
	AddBlock(block *blocks.Block) TipMetadata

	// StrongTips returns the strong tips of the TipManager (with an optional limit).
	StrongTips(optAmount ...int) []TipMetadata

	// WeakTips returns the weak tips of the TipManager (with an optional limit).
	WeakTips(optAmount ...int) []TipMetadata

	// Evict evicts a block from the TipManager.
	Evict(slotIndex iotago.SlotIndex)

	// Events returns the events of the TipManager.
	Events() *Events

	// Interface embeds the required methods of the module.Interface.
	module.Interface
}
