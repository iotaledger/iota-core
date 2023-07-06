package reactive

import (
	"sync"

	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ds/walker"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/core/types"
)

// Set is an agent that can hold and mutate a set of values and that allows other agents to subscribe to updates
// of the set.
//
// The registered callbacks are guaranteed to receive all updates in exactly the same order as they happened and no
// callback is ever more than 1 round of updates ahead of other callbacks.
type Set[ElementType comparable] struct {
	// value is the current value of the set.
	value *advancedset.AdvancedSet[ElementType]

	// updateCallbacks are the registered callbacks that are triggered when the value changes.
	updateCallbacks *shrinkingmap.ShrinkingMap[types.UniqueID, *callback[func(*advancedset.AdvancedSet[ElementType], *SetMutations[ElementType])]]

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
		updateCallbacks: shrinkingmap.New[types.UniqueID, *callback[func(*advancedset.AdvancedSet[T], *SetMutations[T])]](),
	}
}

// Get returns the current value of the set.
func (s *Set[ElementType]) Get() *advancedset.AdvancedSet[ElementType] {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	return s.value
}

// Set sets the given value as the new value of the set.
func (s *Set[ElementType]) Set(value *advancedset.AdvancedSet[ElementType]) (appliedMutations *SetMutations[ElementType]) {
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
func (s *Set[ElementType]) Apply(mutations *SetMutations[ElementType]) (updatedSet *advancedset.AdvancedSet[ElementType], appliedMutations *SetMutations[ElementType]) {
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
func (s *Set[ElementType]) OnUpdate(callback func(updatedSet *advancedset.AdvancedSet[ElementType], appliedMutations *SetMutations[ElementType])) (unsubscribe func()) {
	s.mutex.Lock()

	currentValue := s.value

	newCallback := newCallback[func(*advancedset.AdvancedSet[ElementType], *SetMutations[ElementType])](s.uniqueCallbackID.Next(), callback)
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
func (s *Set[ElementType]) Add(elements *advancedset.AdvancedSet[ElementType]) (updatedSet *advancedset.AdvancedSet[ElementType], appliedMutations *SetMutations[ElementType]) {
	return s.Apply(NewSetMutations(WithAddedElements(elements)))
}

// Remove removes the given elements from the set and returns the updated set and the applied mutations.
func (s *Set[ElementType]) Remove(elements *advancedset.AdvancedSet[ElementType]) (updatedSet *advancedset.AdvancedSet[ElementType], appliedMutations *SetMutations[ElementType]) {
	return s.Apply(NewSetMutations(WithRemovedElements(elements)))
}

// InheritFrom registers the given sets to inherit their mutations to the set.
func (s *Set[ElementType]) InheritFrom(sources ...*Set[ElementType]) (unsubscribe func()) {
	unsubscribeCallbacks := make([]func(), len(sources))

	for i, source := range sources {
		unsubscribeCallbacks[i] = source.OnUpdate(func(_ *advancedset.AdvancedSet[ElementType], appliedMutations *SetMutations[ElementType]) {
			if !appliedMutations.IsEmpty() {
				s.Apply(appliedMutations)
			}
		})
	}

	return lo.Batch(unsubscribeCallbacks...)
}

// Size returns the size of the set.
func (s *Set[ElementType]) Size() int {
	return s.Get().Size()
}

// IsEmpty returns true if the set is empty.
func (s *Set[ElementType]) IsEmpty() bool {
	return s.Get().IsEmpty()
}

// Has returns true if the set contains the given element.
func (s *Set[ElementType]) Has(element ElementType) bool {
	return s.Get().Has(element)
}

// HasAll returns true if the set contains all elements of the other set.
func (s *Set[ElementType]) HasAll(other *Set[ElementType]) bool {
	return s.Get().HasAll(other.Get())
}

// ForEach calls the callback for each element of the set (the iteration can be stopped by returning an error).
func (s *Set[ElementType]) ForEach(callback func(element ElementType) error) error {
	return s.Get().ForEach(callback)
}

// Range calls the callback for each element of the set.
func (s *Set[ElementType]) Range(callback func(element ElementType)) {
	s.Get().Range(callback)
}

// Intersect returns a new set that contains the intersection of the set and the other set.
func (s *Set[ElementType]) Intersect(other *advancedset.AdvancedSet[ElementType]) *advancedset.AdvancedSet[ElementType] {
	return s.Get().Intersect(other)
}

// Filter returns a new set that contains the elements of the set that satisfy the predicate.
func (s *Set[ElementType]) Filter(predicate func(element ElementType) bool) *advancedset.AdvancedSet[ElementType] {
	return s.Get().Filter(predicate)
}

// Equal returns true if the set is equal to the other set.
func (s *Set[ElementType]) Equal(other *advancedset.AdvancedSet[ElementType]) bool {
	return s.Get().Equal(other)
}

// Is returns true if the set contains a single element that is equal to the given element.
func (s *Set[ElementType]) Is(element ElementType) bool {
	return s.Get().Is(element)
}

// Clone returns a shallow copy of the set.
func (s *Set[ElementType]) Clone() *advancedset.AdvancedSet[ElementType] {
	return s.Get().Clone()
}

// Slice returns a slice representation of the set.
func (s *Set[ElementType]) Slice() []ElementType {
	return s.Get().Slice()
}

// Iterator returns an iterator for the set.
func (s *Set[ElementType]) Iterator() *walker.Walker[ElementType] {
	return s.Get().Iterator()
}

// String returns a human-readable version of the set.
func (s *Set[ElementType]) String() string {
	return s.Get().String()
}

// WithTriggerWithInitialEmptyValue is an option that can be set to make the OnUpdate callbacks trigger immediately on
// subscription even if the current value is empty.
func (s *Set[ElementType]) WithTriggerWithInitialEmptyValue(trigger bool) *Set[ElementType] {
	s.optTriggerWithInitialEmptyValue = trigger

	return s
}

// set sets the given value as the new value of the set.
func (s *Set[ElementType]) set(value *advancedset.AdvancedSet[ElementType]) (appliedMutations *SetMutations[ElementType], triggerID types.UniqueID, callbacksToTrigger []*callback[func(*advancedset.AdvancedSet[ElementType], *SetMutations[ElementType])]) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	appliedMutations = NewSetMutations[ElementType](WithRemovedElements(s.value), WithAddedElements(value))
	s.value = value

	return appliedMutations, s.uniqueUpdateID.Next(), s.updateCallbacks.Values()
}

// applyMutations applies the given mutations to the set.
func (s *Set[ElementType]) applyMutations(mutations *SetMutations[ElementType]) (updatedSet *advancedset.AdvancedSet[ElementType], appliedMutations *SetMutations[ElementType], triggerID types.UniqueID, callbacksToTrigger []*callback[func(*advancedset.AdvancedSet[ElementType], *SetMutations[ElementType])]) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

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
	RemovedElements *advancedset.AdvancedSet[T]

	// AddedElements are the elements that are supposed to be added.
	AddedElements *advancedset.AdvancedSet[T]
}

// NewSetMutations creates a new SetMutations instance.
func NewSetMutations[T comparable](opts ...options.Option[SetMutations[T]]) *SetMutations[T] {
	return options.Apply(new(SetMutations[T]), opts, func(s *SetMutations[T]) {
		if s.RemovedElements == nil {
			s.RemovedElements = advancedset.New[T]()
		}

		if s.AddedElements == nil {
			s.AddedElements = advancedset.New[T]()
		}
	})
}

// IsEmpty returns true if the SetMutations instance is empty.
func (s *SetMutations[T]) IsEmpty() bool {
	return s.RemovedElements.IsEmpty() && s.AddedElements.IsEmpty()
}

// WithAddedElements is an option that can be used to set the added elements of a SetMutations instance.
func WithAddedElements[T comparable](elements *advancedset.AdvancedSet[T]) options.Option[SetMutations[T]] {
	return func(args *SetMutations[T]) {
		args.AddedElements = elements
	}
}

// WithRemovedElements is an option that can be used to set the removed elements of a SetMutations instance.
func WithRemovedElements[T comparable](elements *advancedset.AdvancedSet[T]) options.Option[SetMutations[T]] {
	return func(args *SetMutations[T]) {
		args.RemovedElements = elements
	}
}
