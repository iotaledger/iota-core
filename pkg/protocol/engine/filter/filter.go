package filter

import (
	"github.com/iotaledger/hive.go/crypto/identity"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/models"
)

type Filter interface {
	Events() *Events

	// ProcessReceivedBlock processes block from the given source.
	ProcessReceivedBlock(block *models.Block, source identity.ID)

	module.Interface
}
