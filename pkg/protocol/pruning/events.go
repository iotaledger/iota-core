package pruning

import (
	"github.com/iotaledger/hive.go/runtime/event"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Events struct {
	PruningStateChanged *event.Event1[*PruningState]

	event.Group[Events, *Events]
}

var NewEvents = event.CreateGroupConstructor(func() (newEvents *Events) {
	return &Events{
		PruningStateChanged: event.New1[*PruningState](),
	}
})

type PruningState struct {
	IsPruning    bool
	PruningIndex iotago.SlotIndex
}
