package blockgadget

import (
	"github.com/iotaledger/goshimmer/packages/protocol/models"
	"github.com/iotaledger/hive.go/runtime/module"
)

type Gadget interface {
	// IsBlockAccepted returns whether the given block is accepted.
	IsBlockAccepted(blockID models.BlockID) bool

	// IsBlockConfirmed returns whether the given block is confirmed.
	IsBlockConfirmed(blockID models.BlockID) bool

	module.Interface
}
