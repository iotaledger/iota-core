package reactive

import (
	"sync"

	"github.com/iotaledger/hive.go/ds/set"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/core/types"
)

// Set is a reactive Set implementation that allows consumers to subscribe to its changes.
type Set[ElementType comparable] interface {
	// Writeable imports the write methods of the Set interface.
	set.Writeable[ElementType]

	// ReadableSet imports the read methods of the Set interface.
	ReadableSet[ElementType]
}

// NewSet creates a new Set with the given elements.
func NewSet[T comparable](elements ...T) Set[T] {
	return &setImpl[T]{
		readableSet: newReadableSet[T](elements...),
	}
}

// setImpl is the standard implementation of the Set interface.
type setImpl[ElementType comparable] struct {
	// readableSet embeds the ReadableSet implementation.
	*readableSet[ElementType]

	// applyMutex is a mutex that is used to make the Apply method atomic.
	applyMutex sync.Mutex
}

// Add adds a new element to the set and returns true if the element was not present in the set before.
func (s *setImpl[ElementType]) Add(element ElementType) bool {
	return s.Apply(set.NewMutations[ElementType](element)).AddedElements().Has(element)
}

// AddAll adds all elements to the set and returns true if any element has been added.
func (s *setImpl[ElementType]) AddAll(elements set.Readable[ElementType]) (addedElements set.Set[ElementType]) {
	return s.Apply(set.NewMutations[ElementType]().WithAddedElements(elements)).AddedElements()
}

// Delete deletes the given element from the set.
func (s *setImpl[ElementType]) Delete(element ElementType) bool {
	return s.Apply(set.NewMutations[ElementType](element)).DeletedElements().Has(element)
}

// DeleteAll deletes the given elements from the set.
func (s *setImpl[ElementType]) DeleteAll(elements set.Readable[ElementType]) (deletedElements set.Set[ElementType]) {
	return s.Apply(set.NewMutations[ElementType]().WithDeletedElements(elements)).DeletedElements()
}

// Apply applies the given mutations to the set atomically and returns the applied mutations.
func (s *setImpl[ElementType]) Apply(mutations set.Mutations[ElementType]) (appliedMutations set.Mutations[ElementType]) {
	if mutations.IsEmpty() {
		return set.NewMutations[ElementType]()
	}

	s.applyMutex.Lock()
	defer s.applyMutex.Unlock()

	appliedMutations, updateID, registeredCallbacks := s.apply(mutations)
	for _, registeredCallback := range registeredCallbacks {
		if registeredCallback.Lock(updateID) {
			registeredCallback.Invoke(appliedMutations)
			registeredCallback.Unlock()
		}
	}

	return appliedMutations
}

// Decode decodes the set from a byte slice.
func (s *setImpl[ElementType]) Decode(b []byte) (bytesRead int, err error) {
	s.valueMutex.Lock()
	defer s.valueMutex.Unlock()

	return s.value.Decode(b)
}

// ToReadOnly returns a read-only version of the set.
func (s *setImpl[ElementType]) ToReadOnly() set.Readable[ElementType] {
	return s.readableSet
}

// InheritFrom registers the given sets to inherit their mutations to the set.
func (s *setImpl[ElementType]) InheritFrom(sources ...Set[ElementType]) (unsubscribe func()) {
	unsubscribeCallbacks := make([]func(), len(sources))

	for i, source := range sources {
		unsubscribeCallbacks[i] = source.OnUpdate(func(appliedMutations set.Mutations[ElementType]) {
			s.Apply(appliedMutations)
		})
	}

	return lo.Batch(unsubscribeCallbacks...)
}

// apply applies the given mutations to the set.
func (s *setImpl[ElementType]) apply(mutations set.Mutations[ElementType]) (appliedMutations set.Mutations[ElementType], triggerID types.UniqueID, callbacksToTrigger []*callback[func(set.Mutations[ElementType])]) {
	s.valueMutex.Lock()
	defer s.valueMutex.Unlock()

	return s.value.Apply(mutations), s.uniqueUpdateID.Next(), s.updateCallbacks.Values()
}
