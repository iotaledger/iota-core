package chainmanagerv1

import (
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/enginemanager"
)

type EngineManager struct {
	protocol *Protocol

	*enginemanager.EngineManager
}

func NewEngineManager(protocol *Protocol, workers *workerpool.Group, errorHandler func(error), dir string, dbVersion byte, parameters *ProtocolParameters) *EngineManager {
	e := &EngineManager{
		EngineManager: enginemanager.New(
			workers,
			errorHandler,
			dir,
			dbVersion,
			parameters.storageOptions,
			parameters.engineOptions,
			parameters.filterProvider,
			parameters.commitmentFilterProvider,
			parameters.blockDAGProvider,
			parameters.bookerProvider,
			parameters.clockProvider,
			parameters.blockGadgetProvider,
			parameters.slotGadgetProvider,
			parameters.sybilProtectionProvider,
			parameters.notarizationProvider,
			parameters.attestationProvider,
			parameters.ledgerProvider,
			parameters.schedulerProvider,
			parameters.tipManagerProvider,
			parameters.tipSelectionProvider,
			parameters.retainerProvider,
			parameters.upgradeOrchestratorProvider,
		),

		protocol: protocol,
	}

	protocol.chainCreated.Hook(func(chain *Chain) {
		chain.engine.instantiate.OnUpdate(func(_, instantiate bool) {

		})
	})

	return e
}

func (e *EngineManager) MainInstance() *engine.Engine {
	return nil
}
