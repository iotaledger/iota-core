package ledger

import (
	"github.com/iotaledger/hive.go/runtime/event"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Events struct {
	AccountCreated   *event.Event1[iotago.AccountID]
	AccountDestroyed *event.Event1[iotago.AccountID]

	event.Group[Events, *Events]
}

// NewEvents contains the constructor of the Events object (it is generated by a generic factory).
var NewEvents = event.CreateGroupConstructor(func() (newEvents *Events) {
	return &Events{
		AccountCreated:   event.New1[iotago.AccountID](),
		AccountDestroyed: event.New1[iotago.AccountID](),
	}
})
