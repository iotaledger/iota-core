package commitmentfilter

import (
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/iota-core/pkg/model"
)

type Events struct {
	BlockFiltered *event.Event1[*BlockFilteredEvent]
	BlockAllowed  *event.Event1[*model.Block]

	event.Group[Events, *Events]
}

var NewEvents = event.CreateGroupConstructor(func() *Events {
	return &Events{
		BlockFiltered: event.New1[*BlockFilteredEvent](),
		BlockAllowed:  event.New1[*model.Block](),
	}
})

type BlockFilteredEvent struct {
	Block  *model.Block
	Reason error
}
