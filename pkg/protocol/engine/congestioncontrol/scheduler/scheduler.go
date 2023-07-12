package scheduler

import (
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Scheduler interface {
	// AddBlock adds a block to the scheduling buffer.
	AddBlock(*blocks.Block)
	// IsBlockIssuerReady returns true if the block issuer is ready to issuer a block, i.e., if the block issuer were to add a block to the scheduler, would it be scheduled.
	IsBlockIssuerReady(iotago.AccountID, ...*blocks.Block) bool

	module.Interface
}
