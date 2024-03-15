package presolidfilter

import (
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/model"
)

type PreSolidFilter interface {
	// ProcessReceivedBlock processes block from the given source.
	ProcessReceivedBlock(block *model.Block, source peer.ID)

	// Reset resets the component to a clean state as if it was created at the last commitment.
	Reset()

	module.Module
}
