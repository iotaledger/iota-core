package mempool

import (
	"github.com/iotaledger/hive.go/runtime/event"
)

type Events struct {
	TransactionStored *event.Event1[TransactionMetadata]

	TransactionSolid *event.Event1[TransactionMetadata]

	TransactionExecuted *event.Event1[TransactionMetadata]

	TransactionInvalid *event.Event2[TransactionMetadata, error]

	TransactionBooked *event.Event1[TransactionMetadata]

	TransactionAccepted *event.Event1[TransactionMetadata]

	event.Group[Events, *Events]
}

var NewEvents = event.CreateGroupConstructor(func() *Events {
	return &Events{
		TransactionStored:   event.New1[TransactionMetadata](),
		TransactionSolid:    event.New1[TransactionMetadata](),
		TransactionExecuted: event.New1[TransactionMetadata](),
		TransactionInvalid:  event.New2[TransactionMetadata, error](),
		TransactionAccepted: event.New1[TransactionMetadata](),
		TransactionBooked:   event.New1[TransactionMetadata](),
	}
})
