package tipselection

import (
	"time"

	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/model"
)

// TipSelection is a component that is used to abstract away the tip selection strategy, used to issuing new blocks.
type TipSelection interface {
	// SelectTips selects the tips that should be used as references for a new block.
	SelectTips(count int) (references model.ParentReferences)

	// SetAcceptanceTime updates the acceptance time of the TipSelection.
	SetAcceptanceTime(acceptanceTime time.Time) (previousTime time.Time)

	// Reset resets the component to a clean state as if it was created at the last commitment.
	Reset()

	// Interface embeds the required methods of the module.Interface.
	module.Interface
}
