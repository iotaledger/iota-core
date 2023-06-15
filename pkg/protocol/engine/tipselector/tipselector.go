package tipselector

import "github.com/iotaledger/iota-core/pkg/model"

type TipSelector interface {
	// SelectTips selects the tips that should be used for the next block.
	SelectTips(count int) (references model.ParentReferences)
}
