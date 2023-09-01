package chainmanagerv1

import "github.com/iotaledger/hive.go/runtime/workerpool"

type Protocol struct {
	*EngineManager
	*ChainManager
}

func NewProtocol(workers *workerpool.Group, errorHandler func(error), dir string, dbVersion byte, params *ProtocolParameters) *Protocol {
	p := &Protocol{}

	p.EngineManager = NewEngineManager(p, workers.CreateGroup("EngineManager"), errorHandler, dir, dbVersion, params)

	p.ChainManager = NewChainManager(p.EngineManager.MainInstance())

	return p
}
