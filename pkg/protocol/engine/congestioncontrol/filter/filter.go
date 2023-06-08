package filter

import (
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/network"
)

type Filter interface {
	// ProcessReceivedBlock processes block from the given source.
	ProcessReceivedBlock(block *model.Block, source network.PeerID)

	module.Interface
}
