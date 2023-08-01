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
	// Rate returns the specified rate of the Scheduler in work units.
	Rate() int
	// BufferSize returns the current buffer size of the Scheduler as block count.
	BufferSize() int
	// MaxBufferSize returns the max buffer size of the Scheduler as block count.
	MaxBufferSize() int
	// ReadyBlocksCount returns the number of ready blocks.
	ReadyBlocksCount() int
	// IssuerQueueSizeCount returns the queue size of the given issuer as block count.
	IssuerQueueSizeCount(issuerID iotago.AccountID) int
	// IssuerQueueSizeWork returns the queue size of the given issuer in work units.
	IssuerQueueSizeWork(issuerID iotago.AccountID) int

	module.Interface
}
