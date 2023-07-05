package commitmentfilter

import (
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/model"
)

type CommitmentFilter interface {
	// ProcessFilteredBlock processes block from the given source.
	ProcessPreFilteredBlock(block *model.Block)

	module.Interface
}
