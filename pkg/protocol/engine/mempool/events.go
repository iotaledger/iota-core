package mempool

import (
	"github.com/iotaledger/hive.go/runtime/event"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Events struct {
	TransactionStored *event.Event1[TransactionWithMetadata]

	TransactionSolid *event.Event1[TransactionWithMetadata]

	TransactionExecuted *event.Event1[TransactionWithMetadata]

	TransactionInvalid *event.Event2[TransactionWithMetadata, error]

	TransactionBooked *event.Event1[TransactionWithMetadata]

	TransactionAccepted *event.Event1[iotago.TransactionID]

	event.Group[Events, *Events]
}

var NewEvents = event.CreateGroupConstructor(func() *Events {
	return &Events{
		TransactionStored:   event.New1[TransactionWithMetadata](),
		TransactionSolid:    event.New1[TransactionWithMetadata](),
		TransactionExecuted: event.New1[TransactionWithMetadata](),
		TransactionInvalid:  event.New2[TransactionWithMetadata, error](),
		TransactionAccepted: event.New1[iotago.TransactionID](),
		TransactionBooked:   event.New1[TransactionWithMetadata](),
	}
})
