package reactive

import (
	"sync"

	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/core/types"
)

// region Set ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// Set is a reactive Set implementation that allows consumers to subscribe to its changes.
type Set[ElementType comparable] interface {
	// WriteableSet imports the write methods of the Set interface.
	ds.WriteableSet[ElementType]

	// ReadableSet imports the read methods of the Set interface.
	ReadableSet[ElementType]
}

// NewSet creates a new Set with the given elements.
func NewSet[T comparable](elements ...T) Set[T] {
	return newSet(elements...)
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region ReadableSet //////////////////////////////////////////////////////////////////////////////////////////////////

// ReadableSet is a reactive Set implementation that allows consumers to subscribe to its value.
type ReadableSet[ElementType comparable] interface {
	// OnUpdate registers the given callback that is triggered when the value changes.
	OnUpdate(callback func(appliedMutations ds.SetMutations[ElementType]), triggerWithInitialZeroValue ...bool) (unsubscribe func())

	// ReadableSet imports the read methods of the Set interface.
	ds.ReadableSet[ElementType]
}

// NewReadableSet creates a new ReadableSet with the given elements.
func NewReadableSet[ElementType comparable](elements ...ElementType) ReadableSet[ElementType] {
	return newReadableSet(elements...)
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region set //////////////////////////////////////////////////////////////////////////////////////////////////////////

// set is the standard implementation of the Set interface.
type set[ElementType comparable] struct {
	// readableSet embeds the ReadableSet implementation.
	*readableSet[ElementType]

	// applyMutex is a mutex that is used to make the Apply method atomic.
	applyMutex sync.Mutex
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
	return s.Apply(ds.NewSetMutations[ElementType]().WithAddedElements(elements.Clone())).AddedElements()
}

// Delete deletes the given element from the set.
func (s *set[ElementType]) Delete(element ElementType) bool {
	return s.Apply(ds.NewSetMutations[ElementType](element)).DeletedElements().Has(element)
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

	s.applyMutex.Lock()
	defer s.applyMutex.Unlock()

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
	s.applyMutex.Lock()
	defer s.applyMutex.Unlock()

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
	s.valueMutex.Lock()
	defer s.valueMutex.Unlock()

	return s.value.Decode(b)
}

// ReadOnly returns a read-only version of the set.
func (s *set[ElementType]) ReadOnly() ds.ReadableSet[ElementType] {
	return s.readableSet
}

// apply applies the given mutations to the set.
func (s *set[ElementType]) apply(mutations ds.SetMutations[ElementType]) (appliedMutations ds.SetMutations[ElementType], triggerID types.UniqueID, callbacksToTrigger []*callback[func(ds.SetMutations[ElementType])]) {
	s.valueMutex.Lock()
	defer s.valueMutex.Unlock()

	return s.value.Apply(mutations), s.uniqueUpdateID.Next(), s.updateCallbacks.Values()
}

// replace replaces the current value of the set with the given elements.
func (s *set[ElementType]) replace(elements ds.ReadableSet[ElementType]) (appliedMutations ds.SetMutations[ElementType], triggerID types.UniqueID, callbacksToTrigger []*callback[func(ds.SetMutations[ElementType])]) {
	s.valueMutex.Lock()
	defer s.valueMutex.Unlock()

	return ds.NewSetMutations[ElementType](elements.ToSlice()...).WithDeletedElements(s.value.Replace(elements)), s.uniqueUpdateID.Next(), s.updateCallbacks.Values()
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region readableSet //////////////////////////////////////////////////////////////////////////////////////////////////

// readableSet is th standard implementation of the ReadableSet interface.
type readableSet[ElementType comparable] struct {
	// updateCallbacks are the registered callbacks that are triggered when the value changes.
	updateCallbacks *shrinkingmap.ShrinkingMap[types.UniqueID, *callback[func(ds.SetMutations[ElementType])]]

	// uniqueUpdateID is the unique ID that is used to identify an update.
	uniqueUpdateID types.UniqueID

	// uniqueCallbackID is the unique ID that is used to identify a callback.
	uniqueCallbackID types.UniqueID

	// value is the current value of the set.
	value ds.Set[ElementType]

	// valueMutex is the applyMutex that is used to synchronize the access to the value.
	valueMutex sync.RWMutex

	// Readable embeds the set.Readable interface.
	ds.ReadableSet[ElementType]
}

// newReadableSet creates a new readableSet with the given elements.
func newReadableSet[ElementType comparable](elements ...ElementType) *readableSet[ElementType] {
	setInstance := ds.NewSet[ElementType](elements...)

	return &readableSet[ElementType]{
		ReadableSet:     setInstance.ReadOnly(),
		value:           setInstance,
		updateCallbacks: shrinkingmap.New[types.UniqueID, *callback[func(ds.SetMutations[ElementType])]](),
	}
}

// OnUpdate registers the given callback to be triggered when the value of the set changes.
func (r *readableSet[ElementType]) OnUpdate(callback func(appliedMutations ds.SetMutations[ElementType]), triggerWithInitialZeroValue ...bool) (unsubscribe func()) {
	r.valueMutex.Lock()

	mutations := ds.NewSetMutations[ElementType]().WithAddedElements(r.Clone())

	createdCallback := newCallback[func(ds.SetMutations[ElementType])](r.uniqueCallbackID.Next(), callback)
	r.updateCallbacks.Set(createdCallback.ID, createdCallback)

	// grab the lock to make sure that the callback is not executed before we have called it with the initial value.
	createdCallback.LockExecution(r.uniqueUpdateID)
	defer createdCallback.UnlockExecution()

	r.valueMutex.Unlock()

	if !mutations.IsEmpty() || lo.First(triggerWithInitialZeroValue) {
		createdCallback.Invoke(mutations)
	}

	return func() {
		r.updateCallbacks.Delete(createdCallback.ID)

		createdCallback.MarkUnsubscribed()
	}
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////
