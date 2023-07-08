package reactive

import (
	"fmt"
	"sync"

	"github.com/iotaledger/hive.go/ds/set"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/core/types"
)

// Set is a reactive data structure that represents a set of elements. It notifies its subscribers when the set is
// updated.
type Set[ElementType comparable] interface {
	// Get returns the current value of the set.
	Get() set.Set[ElementType]

	// Set sets the new value and triggers the registered callbacks if the value has changed.
	Set(value set.Set[ElementType]) (appliedMutations SetMutations[ElementType])

	// Apply applies the given mutations to the current value and triggers the registered callbacks if the value has
	// changed.
	Apply(mutations SetMutations[ElementType]) (updatedSet set.Set[ElementType], appliedMutations SetMutations[ElementType])

	// OnUpdate registers the given callback that is triggered when the value changes.
	OnUpdate(callback func(updatedSet set.Set[ElementType], appliedMutations SetMutations[ElementType])) (unsubscribe func())

	// Add adds the given elements to the set and triggers the registered callbacks if the value has changed.
	Add(elements set.Set[ElementType]) (updatedSet set.Set[ElementType], appliedMutations SetMutations[ElementType])
	Remove(elements set.Set[ElementType]) (updatedSet set.Set[ElementType], appliedMutations SetMutations[ElementType])
	InheritFrom(sources ...Set[ElementType]) (unsubscribe func())
}

// NewSet is the constructor for the Set type.
func NewSet[T comparable]() Set[T] {
	return &setImpl[T]{
		value:           set.New[T](),
		updateCallbacks: shrinkingmap.New[types.UniqueID, *callback[func(set.Set[T], SetMutations[T])]](),
	}
}

// Set is an agent that can hold and mutate a set of values and that allows other agents to subscribe to updates
// of the set.
//
// The registered callbacks are guaranteed to receive all updates in exactly the same order as they happened and no
// callback is ever more than 1 round of updates ahead of other callbacks.
type setImpl[ElementType comparable] struct {
	// value is the current value of the set.
	value set.Set[ElementType]

	// updateCallbacks are the registered callbacks that are triggered when the value changes.
	updateCallbacks *shrinkingmap.ShrinkingMap[types.UniqueID, *callback[func(set.Set[ElementType], SetMutations[ElementType])]]

	// uniqueUpdateID is the unique ID that is used to identify an update.
	uniqueUpdateID types.UniqueID

	// uniqueCallbackID is the unique ID that is used to identify a callback.
	uniqueCallbackID types.UniqueID

	// mutex is the mutex that is used to synchronize the access to the value.
	mutex sync.RWMutex

	// applyOrderMutex is an additional mutex that is used to ensure that the application order of mutations is ensured.
	applyOrderMutex sync.Mutex
}

// Get returns the current value of the set.
func (s *setImpl[ElementType]) Get() set.Set[ElementType] {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	return s.value
}

// Set sets the given value as the new value of the set.
func (s *setImpl[ElementType]) Set(value set.Set[ElementType]) (appliedMutations SetMutations[ElementType]) {
	s.applyOrderMutex.Lock()
	defer s.applyOrderMutex.Unlock()

	appliedMutations, updateID, callbacksToTrigger := s.set(value)
	for _, callback := range callbacksToTrigger {
		if callback.Lock(updateID) {
			callback.Invoke(value, appliedMutations)
			callback.Unlock()
		}
	}

	return appliedMutations
}

// Apply applies the given SetMutations to the set.
func (s *setImpl[ElementType]) Apply(mutations SetMutations[ElementType]) (updatedSet set.Set[ElementType], appliedMutations SetMutations[ElementType]) {
	s.applyOrderMutex.Lock()
	defer s.applyOrderMutex.Unlock()

	updatedSet, appliedMutations, updateID, callbacksToTrigger := s.applyMutations(mutations)
	for _, callback := range callbacksToTrigger {
		if callback.Lock(updateID) {
			callback.Invoke(updatedSet, appliedMutations)
			callback.Unlock()
		}
	}

	return updatedSet, appliedMutations
}

// OnUpdate registers the given callback to be triggered when the value of the set changes.
func (s *setImpl[ElementType]) OnUpdate(callback func(updatedSet set.Set[ElementType], appliedMutations SetMutations[ElementType])) (unsubscribe func()) {
	s.mutex.Lock()

	currentValue := s.value

	newCallback := newCallback[func(set.Set[ElementType], SetMutations[ElementType])](s.uniqueCallbackID.Next(), callback)
	s.updateCallbacks.Set(newCallback.ID, newCallback)

	// we intertwine the mutexes to ensure that the callback is guaranteed to be triggered with the current value from
	// here first even if the value is updated in parallel.
	newCallback.Lock(s.uniqueUpdateID)
	defer newCallback.Unlock()

	s.mutex.Unlock()

	if !currentValue.IsEmpty() {
		newCallback.Invoke(currentValue, NewSetMutations(WithAddedElements(currentValue)))
	}

	return func() {
		s.updateCallbacks.Delete(newCallback.ID)

		newCallback.MarkUnsubscribed()
	}
}

// Add adds the given elements to the set and returns the updated set and the applied mutations.
func (s *setImpl[ElementType]) Add(elements set.Set[ElementType]) (updatedSet set.Set[ElementType], appliedMutations SetMutations[ElementType]) {
	return s.Apply(NewSetMutations(WithAddedElements(elements)))
}

// Remove removes the given elements from the set and returns the updated set and the applied mutations.
func (s *setImpl[ElementType]) Remove(elements set.Set[ElementType]) (updatedSet set.Set[ElementType], appliedMutations SetMutations[ElementType]) {
	return s.Apply(NewSetMutations(WithRemovedElements(elements)))
}

// InheritFrom registers the given sets to inherit their mutations to the set.
func (s *setImpl[ElementType]) InheritFrom(sources ...Set[ElementType]) (unsubscribe func()) {
	unsubscribeCallbacks := make([]func(), len(sources))

	for i, source := range sources {
		unsubscribeCallbacks[i] = source.OnUpdate(func(_ set.Set[ElementType], appliedMutations SetMutations[ElementType]) {
			if !appliedMutations.IsEmpty() {
				s.Apply(appliedMutations)
			}
		})
	}

	return lo.Batch(unsubscribeCallbacks...)
}

// set sets the given value as the new value of the set.
func (s *setImpl[ElementType]) set(value set.Set[ElementType]) (appliedMutations SetMutations[ElementType], triggerID types.UniqueID, callbacksToTrigger []*callback[func(set.Set[ElementType], SetMutations[ElementType])]) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	appliedMutations = NewSetMutations[ElementType](WithRemovedElements(s.value), WithAddedElements(value))
	s.value = value

	return appliedMutations, s.uniqueUpdateID.Next(), s.updateCallbacks.Values()
}

// applyMutations applies the given mutations to the set.
func (s *setImpl[ElementType]) applyMutations(mutations SetMutations[ElementType]) (updatedSet set.Set[ElementType], appliedMutations SetMutations[ElementType], triggerID types.UniqueID, callbacksToTrigger []*callback[func(set.Set[ElementType], SetMutations[ElementType])]) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	countersPerElement := make(map[ElementType]int)
	fmt.Println("countersPerElement", countersPerElement)

	updatedSet = s.value.Clone()
	appliedMutations = NewSetMutations[ElementType]()

	mutations.RemovedElements.Range(func(element ElementType) {
		if updatedSet.Delete(element) {
			appliedMutations.RemovedElements.Add(element)
		}
	})

	mutations.AddedElements.Range(func(element ElementType) {
		if updatedSet.Add(element) && !appliedMutations.RemovedElements.Delete(element) {
			appliedMutations.AddedElements.Add(element)
		}
	})

	s.value = updatedSet

	return updatedSet, appliedMutations, s.uniqueUpdateID.Next(), s.updateCallbacks.Values()
}

// SetMutations represents an atomic set of mutations that can be applied to a Set.
type SetMutations[T comparable] struct {
	// RemovedElements are the elements that are supposed to be removed.
	RemovedElements set.Set[T]

	// AddedElements are the elements that are supposed to be added.
	AddedElements set.Set[T]
}

// NewSetMutations creates a new SetMutations instance.
func NewSetMutations[T comparable](opts ...options.Option[SetMutations[T]]) SetMutations[T] {
	return *options.Apply(new(SetMutations[T]), opts, func(s *SetMutations[T]) {
		if s.RemovedElements == nil {
			s.RemovedElements = set.New[T]()
		}

		if s.AddedElements == nil {
			s.AddedElements = set.New[T]()
		}
	})
}

// IsEmpty returns true if the SetMutations instance is empty.
func (s SetMutations[T]) IsEmpty() bool {
	return s.RemovedElements.IsEmpty() && s.AddedElements.IsEmpty()
}

// WithAddedElements is an option that can be used to set the added elements of a SetMutations instance.
func WithAddedElements[T comparable](elements set.Set[T]) options.Option[SetMutations[T]] {
	return func(args *SetMutations[T]) {
		args.AddedElements = elements
	}
}

// WithRemovedElements is an option that can be used to set the removed elements of a SetMutations instance.
func WithRemovedElements[T comparable](elements set.Set[T]) options.Option[SetMutations[T]] {
	return func(args *SetMutations[T]) {
		args.RemovedElements = elements
	}
}
