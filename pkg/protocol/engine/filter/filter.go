package filter

import (
	"github.com/iotaledger/hive.go/crypto/identity"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/network/protocols/core"
)

type Filter interface {
	Events() *Events

	// ProcessReceivedBlock processes block from the given source.
	ProcessReceivedBlock(block *core.Block, source identity.ID)

	module.Interface
}
