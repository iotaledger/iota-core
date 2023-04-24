package mempool

import (
	"golang.org/x/xerrors"
	"iota-core/pkg/promise"
	"iota-core/pkg/protocol/engine/ledger"
)

func TypedReferenceResolver(resolvers map[StateReferenceType]StateReferenceResolver) func(StateReference) *promise.Promise[ledger.State] {
	return func(reference StateReference) *promise.Promise[ledger.State] {
		if resolver, resolverExists := resolvers[reference.Type()]; resolverExists {
			return resolver(reference)
		}

		return promise.New[ledger.State]().Reject(xerrors.Errorf("no resolver for type %d", reference.Type()))
	}
}
