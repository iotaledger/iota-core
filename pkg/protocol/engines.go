package protocol

import (
	"fmt"

	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/enginemanager"
)

type Engines struct {
	MainEngineEvents *engine.Events

	protocol *Protocol

	*enginemanager.EngineManager
}

func NewEngines(
	protocol *Protocol,
) *Engines {
	e := &Engines{
		MainEngineEvents: engine.NewEvents(),
		EngineManager: enginemanager.New(
			protocol.Workers,
			func(err error) {
				fmt.Println(err)
			},
			protocol.Options.BaseDirectory,
			3,
			protocol.Options.StorageOptions,
			protocol.Options.EngineOptions,
			protocol.Options.FilterProvider,
			protocol.Options.CommitmentFilterProvider,
			protocol.Options.BlockDAGProvider,
			protocol.Options.BookerProvider,
			protocol.Options.ClockProvider,
			protocol.Options.BlockGadgetProvider,
			protocol.Options.SlotGadgetProvider,
			protocol.Options.SybilProtectionProvider,
			protocol.Options.NotarizationProvider,
			protocol.Options.AttestationProvider,
			protocol.Options.LedgerProvider,
			protocol.Options.SchedulerProvider,
			protocol.Options.TipManagerProvider,
			protocol.Options.TipSelectionProvider,
			protocol.Options.RetainerProvider,
			protocol.Options.UpgradeOrchestratorProvider,
		),

		protocol: protocol,
	}

	protocol.MainEngineR().OnUpdate(func(_, engine *engine.Engine) { e.MainEngineEvents.LinkTo(engine.Events) })

	return e
}

func (e *Engines) MainEngine() *engine.Engine {
	return nil
}

func (e *Engines) MainEngineR() reactive.Variable[*engine.Engine] {
	return nil
}
