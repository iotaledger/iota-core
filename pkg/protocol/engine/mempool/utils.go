package mempool

import (
	"golang.org/x/xerrors"
	"iota-core/pkg/core/promise"
	"iota-core/pkg/protocol/engine/ledger"
)

func TypedReferenceResolver(resolvers map[ledger.StateReferenceType]ledger.StateReferenceResolver) func(ledger.StateReference) *promise.Promise[ledger.State] {
	return func(reference ledger.StateReference) *promise.Promise[ledger.State] {
		if resolver, resolverExists := resolvers[reference.Type()]; resolverExists {
			return resolver(reference)
		}

		return promise.New[ledger.State]().Reject(xerrors.Errorf("no resolver for type %d", reference.Type()))
	}
}
