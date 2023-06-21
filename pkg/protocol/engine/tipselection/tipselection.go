package tipselection

import (
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/model"
)

// TipSelection is a module that is responsible for implementing the tip selection algorithm, that determines which
// blocks to reference when issuing a new block.
type TipSelection interface {
	// SelectTips selects the tips that should be used for issuing the next block.
	SelectTips(count int) (references model.ParentReferences)

	// Interface embeds the required methods of the module.Interface.
	module.Interface
}
