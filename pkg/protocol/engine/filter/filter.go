package filter

import (
	p2ppeer "github.com/libp2p/go-libp2p/core/peer"

	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/model"
)

type Filter interface {
	// ProcessReceivedBlock processes block from the given source.
	ProcessReceivedBlock(block *model.Block, source p2ppeer.ID)

	module.Interface
}
