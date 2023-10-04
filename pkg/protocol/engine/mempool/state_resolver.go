package mempool

import (
	"github.com/iotaledger/iota-core/pkg/core/promise"
)

// StateResolver is a function that resolves a StateReference to a Promise with the State.
type StateResolver func(reference StateReference) *promise.Promise[State]
