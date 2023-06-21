package protocol

import (
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/iota-core/pkg/network/protocols/core"
	"github.com/iotaledger/iota-core/pkg/protocol/chainmanager"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
)

type Events struct {
	Started                  *event.Event
	Stopped                  *event.Event
	CandidateEngineActivated *event.Event1[*engine.Engine]
	MainEngineSwitched       *event.Event1[*engine.Engine]
	Error                    *event.Event1[error]

	Network      *core.Events
	Engine       *engine.Events
	ChainManager *chainmanager.Events
	//Mempool      *mempool.Events

	event.Group[Events, *Events]
}

var NewEvents = event.CreateGroupConstructor(func() (newEvents *Events) {
	return &Events{
		Started:                  event.New(),
		Stopped:                  event.New(),
		CandidateEngineActivated: event.New1[*engine.Engine](),
		MainEngineSwitched:       event.New1[*engine.Engine](),
		Error:                    event.New1[error](),

		Network:      core.NewEvents(),
		Engine:       engine.NewEvents(),
		ChainManager: chainmanager.NewEvents(),
		//Mempool:      mempool.NewEvents(),
	}
})
