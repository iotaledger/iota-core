package filter

import (
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/iota-core/pkg/network/protocols/core"
)

type Events struct {
	BlockFiltered *event.Event1[*BlockFilteredEvent]
	BlockAllowed  *event.Event1[*core.Block]

	event.Group[Events, *Events]
}

var NewEvents = event.CreateGroupConstructor(func() *Events {
	return &Events{
		BlockFiltered: event.New1[*BlockFilteredEvent](),
		BlockAllowed:  event.New1[*core.Block](),
	}
})

type BlockFilteredEvent struct {
	Block  *core.Block
	Reason error
}
