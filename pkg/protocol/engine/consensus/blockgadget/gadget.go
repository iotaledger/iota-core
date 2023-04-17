package blockgadget

import (
	"github.com/iotaledger/hive.go/runtime/module"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Gadget interface {
	// IsBlockAccepted returns whether the given block is accepted.
	IsBlockAccepted(blockID iotago.BlockID) bool

	// IsBlockConfirmed returns whether the given block is confirmed.
	IsBlockConfirmed(blockID iotago.BlockID) bool

	module.Interface
}
