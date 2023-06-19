package tipmanager

import (
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	iotago "github.com/iotaledger/iota.go/v4"
)

// TipManager is a component that maintains a perception of the unreferenced Blocks of the Tangle.
//
// Blocks in the TipManager can be classified to belong to a TipPool which defines how the tip selection strategy
// will treat them:
//
// - StrongTipPool: The block will be referenced via strong parents.
// - WeakTipPool: The block will be referenced via weak parents.
// - DroppedTipPool: The block will be ignored by the tip selection strategy.
//
// The TipManager itself does not classify the blocks, but rather provides a framework to do so. Blocks initially start
// out with an UndefinedTipPool, which causes the block to be temporarily ignored by the tip selection strategy.
//
// The unreferenced blocks of a TipPool form the actual Tips which are used by the tip selection strategy to construct
// new Blocks.
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
