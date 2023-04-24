package mempool

import (
	"iota-core/pkg/protocol/engine/mempool/promise"
	"iota-core/pkg/protocol/engine/vm"
)

// StateReferenceResolver is a function that resolves a StateReference to a State.
type StateReferenceResolver func(reference StateReference) *promise.Promise[vm.State]
