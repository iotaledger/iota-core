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
	// BufferSize returns the current buffer size of the Scheduler as block count.
	BasicBufferSize() int
	// ValidatorBufferSize returns the current buffer size of the Scheduler as block count.
	ValidatorBufferSize() int
	// ReadyBlocksCount returns the number of ready blocks.
	ReadyBlocksCount() int
	// IssuerQueueBlockCount returns the queue size of the given issuer as block count.
	IssuerQueueBlockCount(issuerID iotago.AccountID) int
	// IssuerQueueWork returns the queue size of the given issuer in work units.
	IssuerQueueWork(issuerID iotago.AccountID) iotago.WorkScore
	// ValidatorQueueBlockCount returns the queue size of the given validator as block count.
	ValidatorQueueBlockCount(validatorID iotago.AccountID) int

	module.Interface
}
