package reactive

import (
	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/core/types"
)

// DerivedSet is a reactive Set implementation that allows consumers to subscribe to its changes and that inherits its
// elements from other sets.
type DerivedSet[ElementType comparable] interface {
	Set[ElementType]

	InheritFrom(sources ...ReadableSet[ElementType]) (unsubscribe func())
}

// NewDerivedSet creates a new DerivedSet with the given elements.
func NewDerivedSet[ElementType comparable](elements ...ElementType) DerivedSet[ElementType] {
	return newDerivedSet(elements...)
}

// derivedSet is the standard implementation of the DerivedSet interface.
type derivedSet[ElementType comparable] struct {
	*set[ElementType]

	sourceCounters *shrinkingmap.ShrinkingMap[ElementType, int]
}

// newDerivedSet creates a new derivedSet with the given elements.
func newDerivedSet[ElementType comparable](elements ...ElementType) *derivedSet[ElementType] {
	return &derivedSet[ElementType]{
		set:            newSet(elements...),
		sourceCounters: shrinkingmap.New[ElementType, int](),
	}
}

// InheritFrom registers the given sets to inherit their mutations to the set.
func (s *derivedSet[ElementType]) InheritFrom(sources ...ReadableSet[ElementType]) (unsubscribe func()) {
	unsubscribeCallbacks := make([]func(), 0)

	for _, source := range sources {
		sourceElements := ds.NewSet[ElementType]()

		unsubscribeFromSource := source.OnUpdate(func(appliedMutations ds.SetMutations[ElementType]) {
			s.triggerUpdate(sourceElements.Apply(appliedMutations))
		})

		removeSourceElements := func() {
			s.triggerUpdate(ds.NewSetMutations[ElementType]().WithDeletedElements(sourceElements))
		}

		unsubscribeCallbacks = append(unsubscribeCallbacks, unsubscribeFromSource, removeSourceElements)
	}

	return lo.Batch(unsubscribeCallbacks...)
}

// triggerUpdate triggers the update of the set with the given mutations.
func (s *derivedSet[ElementType]) triggerUpdate(mutations ds.SetMutations[ElementType]) {
	if mutations.IsEmpty() {
		return
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()

	appliedMutations, updateID, registeredCallbacks := s.prepareTrigger(mutations)

	for _, registeredCallback := range registeredCallbacks {
		if registeredCallback.LockExecution(updateID) {
			registeredCallback.Invoke(appliedMutations)
			registeredCallback.UnlockExecution()
		}
	}
}

// prepareTrigger prepares the trigger by applying the given mutations to the set and returning the applied mutations,
// the trigger ID and the callbacks to trigger.
func (s *derivedSet[ElementType]) prepareTrigger(mutations ds.SetMutations[ElementType]) (inheritedMutations ds.SetMutations[ElementType], triggerID types.UniqueID, callbacksToTrigger []*callback[func(ds.SetMutations[ElementType])]) {
	s.valueMutex.Lock()
	defer s.valueMutex.Unlock()

	inheritedMutations = ds.NewSetMutations[ElementType]()

	elementsToAdd := inheritedMutations.AddedElements()
	mutations.AddedElements().Range(func(element ElementType) {
		if s.sourceCounters.Compute(element, func(currentValue int, _ bool) int {
			return currentValue + 1
		}) == 1 {
			elementsToAdd.Add(element)
		}
	})

	elementsToDelete := inheritedMutations.DeletedElements()
	mutations.DeletedElements().Range(func(element ElementType) {
		if s.sourceCounters.Compute(element, func(currentValue int, _ bool) int {
			return currentValue - 1
		}) == 0 && !elementsToAdd.Delete(element) {
			elementsToDelete.Add(element)
		}
	})

	return s.value.Apply(inheritedMutations), s.uniqueUpdateID.Next(), s.updateCallbacks.Values()
}
