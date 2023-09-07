package protocol

import (
	"fmt"

	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/enginemanager"
)

type Engines struct {
	MainEngineEvents *engine.Events

	mainEngine      reactive.Variable[*engine.Engine]
	candidateEngine reactive.Variable[*engine.Engine]

	*enginemanager.EngineManager
}

func newEngines(protocol *Protocol) *Engines {
	e := &Engines{
		MainEngineEvents: engine.NewEvents(),
		EngineManager: enginemanager.New(
			protocol.Workers(),
			func(err error) {
				fmt.Println(err)
			},
			protocol.options.BaseDirectory,
			3,
			protocol.options.StorageOptions,
			protocol.options.EngineOptions,
			protocol.options.FilterProvider,
			protocol.options.CommitmentFilterProvider,
			protocol.options.BlockDAGProvider,
			protocol.options.BookerProvider,
			protocol.options.ClockProvider,
			protocol.options.BlockGadgetProvider,
			protocol.options.SlotGadgetProvider,
			protocol.options.SybilProtectionProvider,
			protocol.options.NotarizationProvider,
			protocol.options.AttestationProvider,
			protocol.options.LedgerProvider,
			protocol.options.SchedulerProvider,
			protocol.options.TipManagerProvider,
			protocol.options.TipSelectionProvider,
			protocol.options.RetainerProvider,
			protocol.options.UpgradeOrchestratorProvider,
			protocol.options.SyncManagerProvider,
		),
		mainEngine:      reactive.NewVariable[*engine.Engine](),
		candidateEngine: reactive.NewVariable[*engine.Engine](),
	}

	if mainEngine, err := e.EngineManager.LoadActiveEngine(protocol.options.SnapshotPath); err != nil {
		panic(fmt.Sprintf("could not load active engine: %s", err))
	} else {
		e.mainEngine.Set(mainEngine)
	}

	protocol.HookConstructed(func() {
		protocol.HeaviestVerifiedCandidate().OnUpdate(func(_, newChain *Chain) {
			e.mainEngine.Set(newChain.Engine())
		})

		e.MainEngineR().OnUpdate(func(_, engine *engine.Engine) { e.MainEngineEvents.LinkTo(engine.Events) })
	})

	return e
}

func (e *Engines) MainEngine() *engine.Engine {
	return e.mainEngine.Get()
}

func (e *Engines) MainEngineR() reactive.Variable[*engine.Engine] {
	return e.mainEngine
}

func (e *Engines) CandidateEngine() *engine.Engine {
	return e.candidateEngine.Get()
}

func (e *Engines) CandidateEngineR() reactive.Variable[*engine.Engine] {
	return e.candidateEngine
}
