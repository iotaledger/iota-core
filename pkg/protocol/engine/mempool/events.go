package mempool

import (
	"github.com/iotaledger/hive.go/runtime/event"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Events struct {
	TransactionAccepted *event.Event1[iotago.TransactionID]

	event.Group[Events, *Events]
}

// NewEvents creates a new Events instance.
var NewEvents = event.CreateGroupConstructor(func() *Events {
	return &Events{
		TransactionAccepted: event.New1[iotago.TransactionID](),
	}
})
