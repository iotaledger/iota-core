package mempool

import (
	"github.com/iotaledger/iota-core/pkg/core/promise"
	iotago "github.com/iotaledger/iota.go/v4"
)

// StateReferenceResolver is a function that resolves a StateReference to a State.
type StateReferenceResolver func(reference iotago.IndexedUTXOReferencer) *promise.Promise[State]
