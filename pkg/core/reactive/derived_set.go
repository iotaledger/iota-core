package reactive

import (
	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/lo"
)

type DerivedSet[ElementType comparable] interface {
	Set[ElementType]

	InheritFrom(sources ...ReadableSet[ElementType]) (unsubscribe func())
}

func NewDerivedSet[ElementType comparable](elements ...ElementType) DerivedSet[ElementType] {
	return newDerivedSet(elements...)
}

type derivedSet[ElementType comparable] struct {
	*set[ElementType]
}

func newDerivedSet[ElementType comparable](elements ...ElementType) *derivedSet[ElementType] {
	return &derivedSet[ElementType]{
		set: newSet(elements...),
	}
}

// InheritFrom registers the given sets to inherit their mutations to the set.
func (s *derivedSet[ElementType]) InheritFrom(sources ...ReadableSet[ElementType]) (unsubscribe func()) {
	unsubscribeCallbacks := make([]func(), len(sources))

	for i, source := range sources {
		unsubscribeCallbacks[i] = source.OnUpdate(func(appliedMutations ds.SetMutations[ElementType]) {
			s.Apply(appliedMutations)
		})
	}

	return lo.Batch(unsubscribeCallbacks...)
}
