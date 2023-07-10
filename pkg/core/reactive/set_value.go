package reactive

import "github.com/iotaledger/hive.go/ds/set"

type SetValue[ElementType comparable] interface {
	// OnUpdate registers the given callback that is triggered when the value changes.
	OnUpdate(callback func(appliedMutations set.Mutations[ElementType])) (unsubscribe func())

	set.ReadOnly[ElementType]
}
