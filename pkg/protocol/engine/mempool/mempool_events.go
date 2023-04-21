package mempool

import (
	"iota-core/pkg/protocol/engine/ledger"

	"github.com/iotaledger/hive.go/runtime/event"
)

type MemPoolEvents struct {
	TransactionStored *event.Event1[TransactionMetadata]

	TransactionSolid *event.Event1[TransactionMetadata]

	TransactionExecuted *event.Event2[TransactionMetadata, []ledger.OutputMetadata]

	TransactionExecutionFailed *event.Event2[TransactionMetadata, error]

	TransactionBooked *event.Event1[TransactionMetadata]

	TransactionAccepted *event.Event1[TransactionID]

	event.Group[MemPoolEvents, *MemPoolEvents]
}

var NewMemPoolEvents = event.CreateGroupConstructor(func() *MemPoolEvents {
	return &MemPoolEvents{
		TransactionStored:          event.New1[TransactionMetadata](),
		TransactionSolid:           event.New1[TransactionMetadata](),
		TransactionExecuted:        event.New2[TransactionMetadata, []ledger.OutputMetadata](),
		TransactionExecutionFailed: event.New2[TransactionMetadata, error](),
		TransactionAccepted:        event.New1[TransactionID](),
		TransactionBooked:          event.New1[TransactionMetadata](),
	}
})
