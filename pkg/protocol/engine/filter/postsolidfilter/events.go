package postsolidfilter

import (
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
)

type Events struct {
	BlockFiltered *event.Event1[*BlockFilteredEvent]
	BlockAllowed  *event.Event1[*blocks.Block]

	event.Group[Events, *Events]
}

var NewEvents = event.CreateGroupConstructor(func() *Events {
	return &Events{
		BlockFiltered: event.New1[*BlockFilteredEvent](),
		BlockAllowed:  event.New1[*blocks.Block](),
	}
})

type BlockFilteredEvent struct {
	Block  *blocks.Block
	Reason error
}
