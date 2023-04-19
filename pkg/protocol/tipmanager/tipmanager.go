package tipmanager

import (
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	iotago "github.com/iotaledger/iota.go/v4"
)

type TipManager interface {
	Events() *Events

	// AddTip adds a Block to the tip pool.
	AddTip(block *blocks.Block) (added bool)

	// RemoveTip removes a Block from the tip pool.
	RemoveTip(blockID iotago.BlockID) (removed bool)

	// Tips returns up to 'count' number of tips.
	Tips(count int) (tips iotago.BlockIDs)

	// AllTips returns all tips contained in the tip pool.
	AllTips() (allTips []*blocks.Block)

	// TipCount returns the total number of tips in the tip pool.
	TipCount() (count int)

	// Shutdown shuts down the TipManager.
	Shutdown()

	module.Interface
}
