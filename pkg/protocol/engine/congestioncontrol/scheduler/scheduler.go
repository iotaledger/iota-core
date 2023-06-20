package scheduler

import (
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Scheduler interface {
	AddBlock(*blocks.Block)
	IsBlockIssuerReady(iotago.AccountID, ...*blocks.Block) bool

	module.Interface
}
