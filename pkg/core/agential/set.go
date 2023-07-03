package agential

import (
	"sync"

	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ds/walker"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/core/types"
)

// Set is a wrapper for an AdvancedSet that is extended by the ability to register callbacks that are
// triggered when the value changes.
type Set[T comparable] struct {
	// value is the current value of the set.
	value *advancedset.AdvancedSet[T]

	// updateCallbacks are the registered callbacks that are triggered when the value changes.
	updateCallbacks *shrinkingmap.ShrinkingMap[types.UniqueID, *Callback[func(*advancedset.AdvancedSet[T], *SetMutations[T])]]

	// uniqueUpdateID is the unique ID that is used to identify an update.
	uniqueUpdateID types.UniqueID

	// uniqueCallbackID is the unique ID that is used to identify a callback.
	uniqueCallbackID types.UniqueID

	// mutex is the mutex that is used to synchronize the access to the value.
	mutex sync.RWMutex

	// applyOrderMutex is an additional mutex that is used to ensure that the application order of mutations is ensured.
	applyOrderMutex sync.Mutex

	// optTriggerWithInitialEmptyValue is an option that can be set to make the OnUpdate callbacks trigger immediately
	// on subscription even if the current value is empty.
	optTriggerWithInitialEmptyValue bool
}

// NewSet is the constructor for the Set type.
func NewSet[T comparable]() *Set[T] {
	return &Set[T]{
		value:           advancedset.New[T](),
		updateCallbacks: shrinkingmap.New[types.UniqueID, *Callback[func(*advancedset.AdvancedSet[T], *SetMutations[T])]](),
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
func (s *Set[T]) Apply(mutations *SetMutations[T]) (updatedSet *advancedset.AdvancedSet[T], appliedMutations *SetMutations[T]) {
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

		newCallback.MarkUnsubscribed()
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

// Size returns the size of the set.
func (s *Set[T]) Size() int {
	return s.Get().Size()
}

// IsEmpty returns true if the set is empty.
func (s *Set[T]) IsEmpty() bool {
	return s.Get().IsEmpty()
}

// Has returns true if the set contains the given element.
func (s *Set[T]) Has(element T) bool {
	return s.Get().Has(element)
}

// HasAll returns true if the set contains all elements of the other set.
func (s *Set[T]) HasAll(other *Set[T]) bool {
	return s.Get().HasAll(other.Get())
}

// ForEach calls the callback for each element of the set (the iteration can be stopped by returning an error).
func (s *Set[T]) ForEach(callback func(element T) error) error {
	return s.Get().ForEach(callback)
}

// Range calls the callback for each element of the set.
func (s *Set[T]) Range(callback func(element T)) {
	s.Get().Range(callback)
}

// Intersect returns a new set that contains the intersection of the set and the other set.
func (s *Set[T]) Intersect(other *advancedset.AdvancedSet[T]) *advancedset.AdvancedSet[T] {
	return s.Get().Intersect(other)
}

// Filter returns a new set that contains the elements of the set that satisfy the predicate.
func (s *Set[T]) Filter(predicate func(element T) bool) *advancedset.AdvancedSet[T] {
	return s.Get().Filter(predicate)
}

// Equal returns true if the set is equal to the other set.
func (s *Set[T]) Equal(other *advancedset.AdvancedSet[T]) bool {
	return s.Get().Equal(other)
}

// Is returns true if the set contains a single element that is equal to the given element.
func (s *Set[T]) Is(element T) bool {
	return s.Get().Is(element)
}

// Clone returns a shallow copy of the set.
func (s *Set[T]) Clone() *advancedset.AdvancedSet[T] {
	return s.Get().Clone()
}

// Slice returns a slice representation of the set.
func (s *Set[T]) Slice() []T {
	return s.Get().Slice()
}

// Iterator returns an iterator for the set.
func (s *Set[T]) Iterator() *walker.Walker[T] {
	return s.Get().Iterator()
}

// String returns a human-readable version of the set.
func (s *Set[T]) String() string {
	return s.Get().String()
}

// WithTriggerWithInitialEmptyValue is an option that can be set to make the OnUpdate callbacks trigger immediately on
// subscription even if the current value is empty.
func (s *Set[T]) WithTriggerWithInitialEmptyValue(trigger bool) *Set[T] {
	s.optTriggerWithInitialEmptyValue = trigger

	return s
}

// set sets the given value as the new value of the set.
func (s *Set[T]) set(value *advancedset.AdvancedSet[T]) (appliedMutations *SetMutations[T], triggerID types.UniqueID, callbacksToTrigger []*Callback[func(*advancedset.AdvancedSet[T], *SetMutations[T])]) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	appliedMutations = NewSetMutations[T](WithRemovedElements(s.value), WithAddedElements(value))
	s.value = value

	return appliedMutations, s.uniqueUpdateID.Next(), s.updateCallbacks.Values()
}

// applyMutations applies the given mutations to the set.
func (s *Set[T]) applyMutations(mutations *SetMutations[T]) (updatedSet *advancedset.AdvancedSet[T], appliedMutations *SetMutations[T], triggerID types.UniqueID, callbacksToTrigger []*Callback[func(*advancedset.AdvancedSet[T], *SetMutations[T])]) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	updatedSet = s.value.Clone()
	appliedMutations = NewSetMutations[T]()

	mutations.RemovedElements.Range(func(element T) {
		if updatedSet.Delete(element) {
			appliedMutations.RemovedElements.Add(element)
		}
	})

	mutations.AddedElements.Range(func(element T) {
		if updatedSet.Add(element) && !appliedMutations.RemovedElements.Delete(element) {
			appliedMutations.AddedElements.Add(element)
		}
	})

	s.value = updatedSet

	return updatedSet, appliedMutations, s.uniqueUpdateID.Next(), s.updateCallbacks.Values()
}
