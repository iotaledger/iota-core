package promise

import (
	"sync"

	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/runtime/options"
)

type Set[T comparable] struct {
	value *advancedset.AdvancedSet[T]

	// updateCallbacks are the registered callbacks that are triggered when the value changes.
	updateCallbacks *shrinkingmap.ShrinkingMap[CallbackID, *setCallback[T]]

	// triggerIDCounter is the counter that is used to assign a unique triggerID to each update.
	triggerIDCounter int

	mutex sync.RWMutex

	// setOrderMutex is an additional mutex that is used to ensure that the order of updates is ensured.
	setOrderMutex sync.Mutex
}

func NewSet[T comparable]() *Set[T] {
	return &Set[T]{
		value:           advancedset.New[T](),
		updateCallbacks: shrinkingmap.New[CallbackID, *setCallback[T]](),
	}
}

func (s *Set[T]) InheritFrom(sources ...*Set[T]) {
	for _, source := range sources {
		source.OnUpdate(func(_ *advancedset.AdvancedSet[T], appliedMutations *SetMutations[T]) {
			if !appliedMutations.IsEmpty() {
				s.Apply(appliedMutations)
			}
		})
	}
}

func (s *Set[T]) Has(element T) bool {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	return s.value.Has(element)
}

func (s *Set[T]) applyMutations(mutations *SetMutations[T]) (updatedSet *advancedset.AdvancedSet[T], appliedMutations *SetMutations[T], triggerID int, callbacksToTrigger []*setCallback[T]) {
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

	s.triggerIDCounter++
	s.value = updatedSet

	return updatedSet, appliedMutations, s.triggerIDCounter, s.updateCallbacks.Values()
}

func (s *Set[T]) Apply(mutations *SetMutations[T]) (updatedSet *advancedset.AdvancedSet[T], appliedMutations *SetMutations[T]) {
	s.setOrderMutex.Lock()
	defer s.setOrderMutex.Unlock()

	updatedSet, appliedMutations, triggerID, callbacksToTrigger := s.applyMutations(mutations)
	for _, callback := range callbacksToTrigger {
		callback.trigger(triggerID, updatedSet, appliedMutations)
	}

	return updatedSet, appliedMutations
}

func (s *Set[T]) Add(elements *advancedset.AdvancedSet[T]) (updatedSet *advancedset.AdvancedSet[T], appliedMutations *SetMutations[T]) {
	return s.Apply(NewSetMutations(WithAddedElements(elements)))
}

func (s *Set[T]) Remove(elements *advancedset.AdvancedSet[T]) (updatedSet *advancedset.AdvancedSet[T], appliedMutations *SetMutations[T]) {
	return s.Apply(NewSetMutations(WithRemovedElements(elements)))
}

func (s *Set[T]) OnUpdate(callback func(updatedSet *advancedset.AdvancedSet[T], appliedMutations *SetMutations[T])) (unsubscribe func()) {
	s.mutex.Lock()

	var (
		currentValue       = s.value
		currentUpdateIndex = s.triggerIDCounter
	)

	createdCallback := newSetCallback[T](callback, currentUpdateIndex)

	s.updateCallbacks.Set(createdCallback.id, createdCallback)

	// we intertwine the mutexes to ensure that the callback is guaranteed to be triggered with the current value from
	// here first even if the value is updated in parallel.
	createdCallback.triggerMutex.Lock()
	defer createdCallback.triggerMutex.Unlock()
	s.mutex.Unlock()

	if !currentValue.IsEmpty() {
		createdCallback.callback(currentValue, NewSetMutations(WithAddedElements(currentValue)))
	}

	return func() {
		s.updateCallbacks.Delete(createdCallback.id)
	}
}

type SetMutations[T comparable] struct {
	RemovedElements *advancedset.AdvancedSet[T]
	AddedElements   *advancedset.AdvancedSet[T]
}

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

func (s *SetMutations[T]) IsEmpty() bool {
	return s.RemovedElements.IsEmpty() && s.AddedElements.IsEmpty()
}

func WithAddedElements[T comparable](elements *advancedset.AdvancedSet[T]) options.Option[SetMutations[T]] {
	return func(args *SetMutations[T]) {
		args.AddedElements = elements
	}
}

func WithRemovedElements[T comparable](elements *advancedset.AdvancedSet[T]) options.Option[SetMutations[T]] {
	return func(args *SetMutations[T]) {
		args.RemovedElements = elements
	}
}

// region setCallback ////////////////////////////////////////////////////////////////////////////////////////////////

// setCallback is a utility struct that holds a callback function and additional information that are required to
// ensure the correct execution order of callbacks in the Value.
type setCallback[T comparable] struct {
	// id is the unique identifier of the callback.
	id CallbackID

	// callback is the function that is executed when the callback is triggered.
	callback func(updatedSet *advancedset.AdvancedSet[T], mutations *SetMutations[T])

	// lastTriggerID is the last triggerID that was used to trigger the callback.
	lastTriggerID int

	// triggerMutex is used to ensure that the callback is only triggered once per triggerID.
	triggerMutex sync.Mutex
}

// newSetCallback creates a new setCallback instance.
func newSetCallback[T comparable](callbackFunc func(updatedSet *advancedset.AdvancedSet[T], mutations *SetMutations[T]), initialUpdateIndex int) *setCallback[T] {
	return &setCallback[T]{
		id:            NewCallbackID(),
		callback:      callbackFunc,
		lastTriggerID: initialUpdateIndex,
	}
}

// trigger triggers the callback if the triggerID is different from the last triggerID.
func (c *setCallback[T]) trigger(triggerID int, updatedSet *advancedset.AdvancedSet[T], mutations *SetMutations[T]) {
	c.triggerMutex.Lock()
	defer c.triggerMutex.Unlock()

	if triggerID != c.lastTriggerID {
		c.lastTriggerID = triggerID

		c.callback(updatedSet, mutations)
	}
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////
