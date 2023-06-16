package tipselection

import (
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/model"
)

type TipSelection interface {
	// SelectTips selects the tips that should be used for the next block.
	SelectTips(count int) (references model.ParentReferences)

	// Interface embeds the required methods of the module.Interface.
	module.Interface
}
