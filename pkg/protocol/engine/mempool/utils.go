package mempool

import (
	"golang.org/x/xerrors"
	"iota-core/pkg/protocol/engine/mempool/promise"
	"iota-core/pkg/protocol/engine/vm"
)

func TypedReferenceResolver(resolvers map[StateReferenceType]StateReferenceResolver) func(StateReference) *promise.Promise[vm.State] {
	return func(reference StateReference) *promise.Promise[vm.State] {
		if resolver, resolverExists := resolvers[reference.Type()]; resolverExists {
			return resolver(reference)
		}

		return promise.New[vm.State]().Reject(xerrors.Errorf("no resolver for type %d", reference.Type()))
	}
}
