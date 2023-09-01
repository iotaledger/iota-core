package protocol

import "github.com/iotaledger/iota-core/pkg/protocol/engine"

type candidateEngine struct {
	engine      *engine.Engine
	cleanupFunc func()
}
