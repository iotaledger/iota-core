package reactive

import (
	"sync"

	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
)

// region set //////////////////////////////////////////////////////////////////////////////////////////////////////////

// set is the standard implementation of the Set interface.
type set[ElementType comparable] struct {
	// readableSet embeds the ReadableSet implementation.
	*readableSet[ElementType]

	// mutex is a mutex that is used to make write operations atomic.
	mutex sync.Mutex
}

// newSet creates a new set with the given elements.
func newSet[ElementType comparable](elements ...ElementType) *set[ElementType] {
	return &set[ElementType]{
		readableSet: newReadableSet[ElementType](elements...),
	}
}

// Add adds a new element to the set and returns true if the element was not present in the set before.
func (s *set[ElementType]) Add(element ElementType) bool {
	return s.Apply(ds.NewSetMutations[ElementType](element)).AddedElements().Has(element)
}

// AddAll adds all elements to the set and returns true if any element has been added.
func (s *set[ElementType]) AddAll(elements ds.ReadableSet[ElementType]) (addedElements ds.Set[ElementType]) {
	return s.Apply(ds.NewSetMutations[ElementType](elements.ToSlice()...)).AddedElements()
}

// Delete deletes the given element from the set.
func (s *set[ElementType]) Delete(element ElementType) bool {
	return s.Apply(ds.NewSetMutations[ElementType]().WithDeletedElements(ds.NewSet(element))).DeletedElements().Has(element)
}

// DeleteAll deletes the given elements from the set.
func (s *set[ElementType]) DeleteAll(elements ds.ReadableSet[ElementType]) (deletedElements ds.Set[ElementType]) {
	return s.Apply(ds.NewSetMutations[ElementType]().WithDeletedElements(elements.Clone())).DeletedElements()
}

// Apply applies the given mutations to the set atomically and returns the applied mutations.
func (s *set[ElementType]) Apply(mutations ds.SetMutations[ElementType]) (appliedMutations ds.SetMutations[ElementType]) {
	if mutations.IsEmpty() {
		return ds.NewSetMutations[ElementType]()
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()

	appliedMutations, updateID, registeredCallbacks := s.apply(mutations)

	for _, registeredCallback := range registeredCallbacks {
		if registeredCallback.LockExecution(updateID) {
			registeredCallback.Invoke(appliedMutations)
			registeredCallback.UnlockExecution()
		}
	}

	return appliedMutations
}

// Replace replaces the current value of the set with the given elements.
func (s *set[ElementType]) Replace(elements ds.ReadableSet[ElementType]) (removedElements ds.Set[ElementType]) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	appliedMutations, updateID, registeredCallbacks := s.replace(elements)

	for _, registeredCallback := range registeredCallbacks {
		if registeredCallback.LockExecution(updateID) {
			registeredCallback.Invoke(appliedMutations)
			registeredCallback.UnlockExecution()
		}
	}

	return appliedMutations.DeletedElements()
}

// Decode decodes the set from a byte slice.
func (s *set[ElementType]) Decode(b []byte) (bytesRead int, err error) {
	s.readableSet.mutex.Lock()
	defer s.readableSet.mutex.Unlock()

	return s.value.Decode(b)
}

// ReadOnly returns a read-only version of the set.
func (s *set[ElementType]) ReadOnly() ds.ReadableSet[ElementType] {
	return s.readableSet
}

// apply applies the given mutations to the set.
func (s *set[ElementType]) apply(mutations ds.SetMutations[ElementType]) (appliedMutations ds.SetMutations[ElementType], triggerID uniqueID, callbacksToTrigger []*callback[func(ds.SetMutations[ElementType])]) {
	s.readableSet.mutex.Lock()
	defer s.readableSet.mutex.Unlock()

	return s.value.Apply(mutations), s.uniqueUpdateID.Next(), s.updateCallbacks.Values()
}

// replace replaces the current value of the set with the given elements.
func (s *set[ElementType]) replace(elements ds.ReadableSet[ElementType]) (appliedMutations ds.SetMutations[ElementType], triggerID uniqueID, callbacksToTrigger []*callback[func(ds.SetMutations[ElementType])]) {
	s.readableSet.mutex.Lock()
	defer s.readableSet.mutex.Unlock()

	return ds.NewSetMutations[ElementType](elements.ToSlice()...).WithDeletedElements(s.value.Replace(elements)), s.uniqueUpdateID.Next(), s.updateCallbacks.Values()
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region readableSet //////////////////////////////////////////////////////////////////////////////////////////////////

// readableSet is th standard implementation of the ReadableSet interface.
type readableSet[ElementType comparable] struct {
	// updateCallbacks are the registered callbacks that are triggered when the value changes.
	updateCallbacks *shrinkingmap.ShrinkingMap[uniqueID, *callback[func(ds.SetMutations[ElementType])]]

	// uniqueUpdateID is the unique ID that is used to identify an update.
	uniqueUpdateID uniqueID

	// uniqueCallbackID is the unique ID that is used to identify a callback.
	uniqueCallbackID uniqueID

	// value is the current value of the set.
	value ds.Set[ElementType]

	// mutex is the mutex that is used to synchronize the access to the value.
	mutex sync.RWMutex

	// Readable embeds the set.Readable interface.
	ds.ReadableSet[ElementType]
}

// newReadableSet creates a new readableSet with the given elements.
func newReadableSet[ElementType comparable](elements ...ElementType) *readableSet[ElementType] {
	setInstance := ds.NewSet[ElementType](elements...)

	return &readableSet[ElementType]{
		ReadableSet:     setInstance.ReadOnly(),
		value:           setInstance,
		updateCallbacks: shrinkingmap.New[uniqueID, *callback[func(ds.SetMutations[ElementType])]](),
	}
}

// OnUpdate registers the given callback to be triggered when the value of the set changes.
func (r *readableSet[ElementType]) OnUpdate(callback func(appliedMutations ds.SetMutations[ElementType]), triggerWithInitialZeroValue ...bool) (unsubscribe func()) {
	r.mutex.Lock()

	mutations := ds.NewSetMutations[ElementType]().WithAddedElements(r.Clone())

	createdCallback := newCallback[func(ds.SetMutations[ElementType])](r.uniqueCallbackID.Next(), callback)
	r.updateCallbacks.Set(createdCallback.ID, createdCallback)

	// grab the lock to make sure that the callback is not executed before we have called it with the initial value.
	createdCallback.LockExecution(r.uniqueUpdateID)
	defer createdCallback.UnlockExecution()

	r.mutex.Unlock()

	if !mutations.IsEmpty() || lo.First(triggerWithInitialZeroValue) {
		createdCallback.Invoke(mutations)
	}

	return func() {
		r.updateCallbacks.Delete(createdCallback.ID)

		createdCallback.MarkUnsubscribed()
	}
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region derivedSet ///////////////////////////////////////////////////////////////////////////////////////////////////

// derivedSet is the standard implementation of the DerivedSet interface.
type derivedSet[ElementType comparable] struct {
	// set is the set that is derived from the source sets.
	*set[ElementType]

	// sourceCounters are the counters that keep track of the number of times an element is contained in the source
	// sets (we only want to remove an element from the set if it is not contained in any of the source sets anymore).
	sourceCounters *shrinkingmap.ShrinkingMap[ElementType, int]
}

// newDerivedSet creates a new derivedSet with the given elements.
func newDerivedSet[ElementType comparable]() *derivedSet[ElementType] {
	return &derivedSet[ElementType]{
		set:            newSet[ElementType](),
		sourceCounters: shrinkingmap.New[ElementType, int](),
	}
}

// InheritFrom registers the given sets to inherit their mutations to the set.
func (s *derivedSet[ElementType]) InheritFrom(sources ...ReadableSet[ElementType]) (unsubscribe func()) {
	unsubscribeCallbacks := make([]func(), 0)

	for _, source := range sources {
		sourceElements := ds.NewSet[ElementType]()

		unsubscribeFromSource := source.OnUpdate(func(appliedMutations ds.SetMutations[ElementType]) {
			s.Apply(sourceElements.Apply(appliedMutations))
		})

		removeSourceElements := func() {
			s.Apply(ds.NewSetMutations[ElementType]().WithDeletedElements(sourceElements))
		}

		unsubscribeCallbacks = append(unsubscribeCallbacks, unsubscribeFromSource, removeSourceElements)
	}

	return lo.Batch(unsubscribeCallbacks...)
}

// Apply triggers the update of the set with the given mutations.
func (s *derivedSet[ElementType]) Apply(mutations ds.SetMutations[ElementType]) (appliedMutations ds.SetMutations[ElementType]) {
	if mutations.IsEmpty() {
		return
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()

	appliedMutations, updateID, registeredCallbacks := s.apply(mutations)

	for _, registeredCallback := range registeredCallbacks {
		if registeredCallback.LockExecution(updateID) {
			registeredCallback.Invoke(appliedMutations)
			registeredCallback.UnlockExecution()
		}
	}

	return appliedMutations
}

// apply prepares the trigger by applying the given mutations to the set and returning the applied
// mutations, the trigger ID and the callbacks to trigger.
func (s *derivedSet[ElementType]) apply(mutations ds.SetMutations[ElementType]) (inheritedMutations ds.SetMutations[ElementType], triggerID uniqueID, callbacksToTrigger []*callback[func(ds.SetMutations[ElementType])]) {
	s.readableSet.mutex.Lock()
	defer s.readableSet.mutex.Unlock()

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

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////
