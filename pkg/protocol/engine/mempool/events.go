package mempool

import (
	"github.com/iotaledger/hive.go/runtime/event"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Events struct {
	TransactionStored *event.Event1[TransactionMetadata]

	TransactionSolid *event.Event1[TransactionMetadata]

	TransactionExecuted *event.Event1[TransactionMetadata]

	TransactionExecutionFailed *event.Event2[TransactionMetadata, error]

	TransactionBooked *event.Event1[TransactionMetadata]

	TransactionAccepted *event.Event1[iotago.TransactionID]

	event.Group[Events, *Events]
}

var NewEvents = event.CreateGroupConstructor(func() *Events {
	return &Events{
		TransactionStored:          event.New1[TransactionMetadata](),
		TransactionSolid:           event.New1[TransactionMetadata](),
		TransactionExecuted:        event.New1[TransactionMetadata](),
		TransactionExecutionFailed: event.New2[TransactionMetadata, error](),
		TransactionAccepted:        event.New1[iotago.TransactionID](),
		TransactionBooked:          event.New1[TransactionMetadata](),
	}
})
