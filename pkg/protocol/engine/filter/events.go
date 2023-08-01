package filter

import (
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/network"
)

type Events struct {
	BlockPreFiltered *event.Event1[*BlockPreFilteredEvent]
	BlockPreAllowed  *event.Event1[*model.Block]

	event.Group[Events, *Events]
}

var NewEvents = event.CreateGroupConstructor(func() *Events {
	return &Events{
		BlockPreFiltered: event.New1[*BlockPreFilteredEvent](),
		BlockPreAllowed:  event.New1[*model.Block](),
	}
})

type BlockPreFilteredEvent struct {
	Block  *model.Block
	Reason error
	Source network.PeerID
}
