package reactive

import (
	"sync"

	"github.com/iotaledger/hive.go/ds/set"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/core/types"
)

// Set is a reactive data structure that represents a set of elements. It notifies its subscribers when the set is
// updated.
type Set[ElementType comparable] interface {
	// Add adds a new element to the set and returns true if the element was not present in the set before.
	Add(element ElementType) bool

	// AddAll adds all elements to the set and returns true if any element has been added.
	AddAll(elements set.ReadOnly[ElementType]) (addedElements set.Set[ElementType])

	// Delete deletes the given element from the set.
	Delete(element ElementType) bool

	// DeleteAll deletes the given elements from the set.
	DeleteAll(other set.ReadOnly[ElementType]) (removedElements set.Set[ElementType])

	// Apply tries to apply the given mutations to the set atomically and returns the applied mutations.
	Apply(mutations set.Mutations[ElementType]) (appliedMutations set.Mutations[ElementType])

	// Decode decodes the set from a byte slice.
	Decode(b []byte) (bytesRead int, err error)

	// ToReadOnly returns a read-only version of the set.
	ToReadOnly() set.ReadOnly[ElementType]

	// Add adds the given elements to the set and triggers the registered callbacks if the value has changed.
	// InheritFrom(sources ...Set[ElementType]) (unsubscribe func())

	SetValue[ElementType]
}

// NewSet is the constructor for the Set type.
func NewSet[T comparable]() Set[T] {
	return &setImpl[T]{
		Set:             set.New[T](),
		updateCallbacks: shrinkingmap.New[types.UniqueID, *callback[func(set.Mutations[T])]](),
	}
}

// Set is an agent that can hold and mutate a set of values and that allows other agents to subscribe to updates
// of the set.
//
// The registered callbacks are guaranteed to receive all updates in exactly the same order as they happened and no
// callback is ever more than 1 round of updates ahead of other callbacks.
type setImpl[ElementType comparable] struct {
	// value is the current value of the set.
	set.Set[ElementType]

	// updateCallbacks are the registered callbacks that are triggered when the value changes.
	updateCallbacks *shrinkingmap.ShrinkingMap[types.UniqueID, *callback[func(set.Mutations[ElementType])]]

	// uniqueUpdateID is the unique ID that is used to identify an update.
	uniqueUpdateID types.UniqueID

	// uniqueCallbackID is the unique ID that is used to identify a callback.
	uniqueCallbackID types.UniqueID

	// mutex is the mutex that is used to synchronize the access to the value.
	mutex sync.RWMutex

	// applyOrderMutex is an additional mutex that is used to ensure that the application order of mutations is ensured.
	applyOrderMutex sync.Mutex
}

func (s *setImpl[ElementType]) Add(element ElementType) bool {
	mutations := set.NewMutations[ElementType]()
	mutations.AddedElements().Add(element)

	return s.Apply(mutations).AddedElements().Has(element)

}

// Apply applies the given set.Mutations[] to the set.
func (s *setImpl[ElementType]) Apply(mutations set.Mutations[ElementType]) (appliedMutations set.Mutations[ElementType]) {
	s.applyOrderMutex.Lock()
	defer s.applyOrderMutex.Unlock()

	appliedMutations, updateID, callbacksToTrigger := s.applyMutations(mutations)
	for _, callback := range callbacksToTrigger {
		if callback.Lock(updateID) {
			callback.Invoke(appliedMutations)
			callback.Unlock()
		}
	}

	return appliedMutations
}

// OnUpdate registers the given callback to be triggered when the value of the set changes.
func (s *setImpl[ElementType]) OnUpdate(callback func(appliedMutations set.Mutations[ElementType])) (unsubscribe func()) {
	s.mutex.Lock()

	currentValue := set.NewMutations[ElementType]().WithAddedElements(s.Set)

	newCallback := newCallback[func(set.Mutations[ElementType])](s.uniqueCallbackID.Next(), callback)
	s.updateCallbacks.Set(newCallback.ID, newCallback)

	// we intertwine the mutexes to ensure that the callback is guaranteed to be triggered with the current value from
	// here first even if the value is updated in parallel.
	newCallback.Lock(s.uniqueUpdateID)
	defer newCallback.Unlock()

	s.mutex.Unlock()

	if !currentValue.IsEmpty() {
		newCallback.Invoke(currentValue)
	}

	return func() {
		s.updateCallbacks.Delete(newCallback.ID)

		newCallback.MarkUnsubscribed()
	}
}

func (s *setImpl[ElementType]) AddAll(elements set.ReadOnly[ElementType]) (addedElements set.Set[ElementType]) {
	return s.Apply(set.NewMutations[ElementType]().WithAddedElements(elements)).AddedElements()
}

func (s *setImpl[ElementType]) DeleteAll(elements set.ReadOnly[ElementType]) (deletedElements set.Set[ElementType]) {
	return s.Apply(set.NewMutations[ElementType]().WithDeletedElements(elements)).DeletedElements()
}

// InheritFrom registers the given sets to inherit their mutations to the set.
func (s *setImpl[ElementType]) InheritFrom(sources ...Set[ElementType]) (unsubscribe func()) {
	unsubscribeCallbacks := make([]func(), len(sources))

	for i, source := range sources {
		unsubscribeCallbacks[i] = source.OnUpdate(func(appliedMutations set.Mutations[ElementType]) {
			if !appliedMutations.IsEmpty() {
				s.Apply(appliedMutations)
			}
		})
	}

	return lo.Batch(unsubscribeCallbacks...)
}

// applyMutations applies the given mutations to the set.
func (s *setImpl[ElementType]) applyMutations(mutations set.Mutations[ElementType]) (appliedMutations set.Mutations[ElementType], triggerID types.UniqueID, callbacksToTrigger []*callback[func(set.Mutations[ElementType])]) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	appliedMutations = set.NewMutations[ElementType]()

	mutations.DeletedElements().Range(func(element ElementType) {
		if s.Set.Delete(element) {
			appliedMutations.DeletedElements().Add(element)
		}
	})

	mutations.AddedElements().Range(func(element ElementType) {
		if s.Set.Add(element) && !appliedMutations.DeletedElements().Delete(element) {
			appliedMutations.AddedElements().Add(element)
		}
	})

	return appliedMutations, s.uniqueUpdateID.Next(), s.updateCallbacks.Values()
}
