package promise

import (
	"sync"

	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
)

// Set is a wrapper for an AdvancedSet that is extended by the ability to register callbacks that are
// triggered when the value changes.
type Set[T comparable] struct {
	// value is the current value of the set.
	value *advancedset.AdvancedSet[T]

	// updateCallbacks are the registered callbacks that are triggered when the value changes.
	updateCallbacks *shrinkingmap.ShrinkingMap[UniqueID, *Callback[func(*advancedset.AdvancedSet[T], *SetMutations[T])]]

	// uniqueUpdateID is the unique ID that is used to identify an update.
	uniqueUpdateID UniqueID

	// uniqueCallbackID is the unique ID that is used to identify a callback.
	uniqueCallbackID UniqueID

	// mutex is the mutex that is used to synchronize the access to the value.
	mutex sync.RWMutex

	// applyMutex is an additional mutex that is used to ensure that the application order of mutations is ensured.
	applyMutex sync.Mutex
}

// NewSet is the constructor for the Set type.
func NewSet[T comparable]() *Set[T] {
	return &Set[T]{
		value:           advancedset.New[T](),
		updateCallbacks: shrinkingmap.New[UniqueID, *Callback[func(*advancedset.AdvancedSet[T], *SetMutations[T])]](),
	}
}

// Get returns the current value of the set.
func (s *Set[T]) Get() *advancedset.AdvancedSet[T] {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	return s.value
}

// Set sets the given value as the new value of the set.
func (s *Set[T]) Set(value *advancedset.AdvancedSet[T]) (appliedMutations *SetMutations[T]) {
	s.applyMutex.Lock()
	defer s.applyMutex.Unlock()

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
func (s *Set[T]) Apply(mutations *SetMutations[T]) (updatedSet *advancedset.AdvancedSet[T], appliedMutations *SetMutations[T]) {
	s.applyMutex.Lock()
	defer s.applyMutex.Unlock()

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
func (s *Set[T]) OnUpdate(callback func(updatedSet *advancedset.AdvancedSet[T], appliedMutations *SetMutations[T])) (unsubscribe func()) {
	s.mutex.Lock()

	currentValue := s.value

	newCallback := NewCallback[func(*advancedset.AdvancedSet[T], *SetMutations[T])](s.uniqueCallbackID.Next(), callback)
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
	}
}

// Add adds the given elements to the set and returns the updated set and the applied mutations.
func (s *Set[T]) Add(elements *advancedset.AdvancedSet[T]) (updatedSet *advancedset.AdvancedSet[T], appliedMutations *SetMutations[T]) {
	return s.Apply(NewSetMutations(WithAddedElements(elements)))
}

// Remove removes the given elements from the set and returns the updated set and the applied mutations.
func (s *Set[T]) Remove(elements *advancedset.AdvancedSet[T]) (updatedSet *advancedset.AdvancedSet[T], appliedMutations *SetMutations[T]) {
	return s.Apply(NewSetMutations(WithRemovedElements(elements)))
}

// Has returns true if the set contains the given element.
func (s *Set[T]) Has(element T) bool {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	return s.value.Has(element)
}

// InheritFrom registers the given sets to inherit their mutations to the set.
func (s *Set[T]) InheritFrom(sources ...*Set[T]) (unsubscribe func()) {
	unsubscribeCallbacks := make([]func(), len(sources))

	for i, source := range sources {
		unsubscribeCallbacks[i] = source.OnUpdate(func(_ *advancedset.AdvancedSet[T], appliedMutations *SetMutations[T]) {
			if !appliedMutations.IsEmpty() {
				s.Apply(appliedMutations)
			}
		})
	}

	return lo.Batch(unsubscribeCallbacks...)
}

func (s *Set[T]) set(value *advancedset.AdvancedSet[T]) (appliedMutations *SetMutations[T], triggerID UniqueID, callbacksToTrigger []*Callback[func(*advancedset.AdvancedSet[T], *SetMutations[T])]) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	appliedMutations = NewSetMutations[T](WithRemovedElements(s.value), WithAddedElements(value))
	s.value = value

	return appliedMutations, s.uniqueUpdateID.Next(), s.updateCallbacks.Values()
}

// applyMutations applies the given mutations to the set.
func (s *Set[T]) applyMutations(mutations *SetMutations[T]) (updatedSet *advancedset.AdvancedSet[T], appliedMutations *SetMutations[T], triggerID UniqueID, callbacksToTrigger []*Callback[func(*advancedset.AdvancedSet[T], *SetMutations[T])]) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	updatedSet = s.value.Clone()
	appliedMutations = NewSetMutations[T]()

	mutations.RemovedElements.ForEach(func(element T) error {
		if updatedSet.Delete(element) {
			appliedMutations.RemovedElements.Add(element)
		}

		return nil
	})

	mutations.AddedElements.ForEach(func(element T) error {
		if updatedSet.Add(element) && !appliedMutations.RemovedElements.Delete(element) {
			appliedMutations.AddedElements.Add(element)
		}

		return nil
	})

	s.value = updatedSet

	return updatedSet, appliedMutations, s.uniqueUpdateID.Next(), s.updateCallbacks.Values()
}
