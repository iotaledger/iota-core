package commitmentfilter

import (
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/model"
)

type CommitmentFilter interface {
	// ProcessPreFilteredBlock processes block from the given source.
	ProcessPreFilteredBlock(block *model.Block)

	module.Interface
}
