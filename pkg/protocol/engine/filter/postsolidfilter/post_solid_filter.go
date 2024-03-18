package postsolidfilter

import (
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
)

type PostSolidFilter interface {
	// ProcessSolidBlock processes block from the given source.
	ProcessSolidBlock(block *blocks.Block)

	// Reset resets the component to a clean state as if it was created at the last commitment.
	Reset()

	module.Module
}
