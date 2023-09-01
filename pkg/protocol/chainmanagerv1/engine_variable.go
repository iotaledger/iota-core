package chainmanagerv1

import (
	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
)

type engineVariable struct {
	reactive.Variable[*engine.Engine]

	parentEngine reactive.Variable[*engine.Engine]

	spawnedEngine reactive.Variable[*engine.Engine]

	// instantiated is a flag that indicates whether this chain shall be instantiated.
	instantiate reactive.Variable[bool]
}

func newEngineVariable(commitment *Commitment) *engineVariable {
	e := &engineVariable{
		parentEngine:  reactive.NewVariable[*engine.Engine](),
		instantiate:   reactive.NewVariable[bool](),
		spawnedEngine: reactive.NewVariable[*engine.Engine](),
	}

	commitment.parent.OnUpdate(func(_, parent *Commitment) {
		e.parentEngine.InheritFrom(parent.Engine())
	})

	e.Variable = reactive.NewDerivedVariable2(func(spawnedEngine, parentEngine *engine.Engine) *engine.Engine {
		if spawnedEngine != nil {
			return spawnedEngine
		}

		return parentEngine
	}, e.spawnedEngine, e.parentEngine)

	return e
}
