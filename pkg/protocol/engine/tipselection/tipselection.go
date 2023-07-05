package tipselection

import (
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/model"
)

// TipSelection is a component that is used to abstract away the tip selection strategy, used to issuing new blocks.
type TipSelection interface {
	// SelectTips selects the tips that should be used as references for a new block.
	SelectTips(count int) (references model.ParentReferences)

	// Interface embeds the required methods of the module.Interface.
	module.Interface
}
