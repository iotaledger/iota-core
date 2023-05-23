package promise

import (
	"sync"

	"github.com/iotaledger/hive.go/ds/shrinkingmap"
)

// region Value ////////////////////////////////////////////////////////////////////////////////////////////////////////

// Value is a thread safe value that allows multiple consumers to subscribe to changes by registering callbacks that are
// executed when the value is updated.
//
// The registered callbacks are guaranteed to receive all updates in exactly the same order as they happened, but
// there is no guarantee about the order of updates between different callbacks (they are executed in a random order).
//
// It is however guaranteed, that no callback will be ahead of other callbacks by more than 1 "round".
type Value[T comparable] struct {
	// value is the current value.
	value T

	// updateCallbacks are the registered callbacks that are triggered when the value changes.
	updateCallbacks *shrinkingmap.ShrinkingMap[UniqueID, *valueCallback[T]]

	callbackIDs UniqueID

	// triggerIDCounter is the counter that is used to assign a unique triggerID to each update.
	triggerIDCounter int

	// mutex is used to ensure that updating the value and registering/unregistering callbacks is thread safe.
	mutex sync.RWMutex

	// setOrderMutex is an additional mutex that is used to ensure that the order of updates is ensured.
	setOrderMutex sync.Mutex

	// optTriggerWithInitialEmptyValue is an option that can be set to true to trigger the callbacks even when the
	// initial value is empty on subscription.
	optTriggerWithInitialEmptyValue bool
}

// NewValue creates a new Value instance.
func NewValue[T comparable]() *Value[T] {
	return &Value[T]{
		updateCallbacks: shrinkingmap.New[UniqueID, *valueCallback[T]](),
	}
}

// Get returns the current value.
func (v *Value[T]) Get() T {
	v.mutex.RLock()
	defer v.mutex.RUnlock()

	return v.value
}

// Set sets the new value and triggers the registered callbacks if the value has changed.
func (v *Value[T]) Set(newValue T) (previousValue T) {
	return v.Compute(func(T) T { return newValue })
}

// Compute computes the new value based on the current value and triggers the registered callbacks if the value changed.
func (v *Value[T]) Compute(computeFunc func(currentValue T) T) (previousValue T) {
	v.setOrderMutex.Lock()
	defer v.setOrderMutex.Unlock()

	newValue, previousValue, triggerID, callbacksToTrigger := v.prepareDynamicTrigger(computeFunc)
	for _, callback := range callbacksToTrigger {
		callback.trigger(triggerID, previousValue, newValue)
	}

	return previousValue
}

func (v *Value[T]) WithTriggerWithInitialEmptyValue(trigger bool) *Value[T] {
	v.optTriggerWithInitialEmptyValue = trigger

	return v
}

// OnUpdate registers a callback that is triggered when the value changes.
func (v *Value[T]) OnUpdate(callback func(prevValue, newValue T)) (unsubscribe func()) {
	v.mutex.Lock()

	var (
		previousValue      T
		currentValue       = v.value
		currentUpdateIndex = v.triggerIDCounter
	)

	createdCallback := newValueCallback[T](v.callbackIDs.Next(), callback, currentUpdateIndex)

	v.updateCallbacks.Set(createdCallback.id, createdCallback)

	// we intertwine the mutexes to ensure that the callback is guaranteed to be triggered with the current value from
	// here first even if the value is updated in parallel.
	createdCallback.triggerMutex.Lock()
	defer createdCallback.triggerMutex.Unlock()
	v.mutex.Unlock()

	if v.optTriggerWithInitialEmptyValue || previousValue != currentValue {
		createdCallback.callback(previousValue, currentValue)
	}

	return func() {
		v.updateCallbacks.Delete(createdCallback.id)
	}
}

// prepareDynamicTrigger atomically prepares the trigger by setting the new value and returning the previous value, the
// triggerID and the callbacks to trigger.
func (v *Value[T]) prepareDynamicTrigger(newValueGenerator func(T) T) (newValue, previousValue T, triggerID int, callbacksToTrigger []*valueCallback[T]) {
	v.mutex.Lock()
	defer v.mutex.Unlock()

	if previousValue, newValue = v.value, newValueGenerator(previousValue); newValue == previousValue {
		return newValue, previousValue, 0, nil
	}

	v.triggerIDCounter++
	v.value = newValue

	return newValue, previousValue, v.triggerIDCounter, v.updateCallbacks.Values()
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region valueCallback ////////////////////////////////////////////////////////////////////////////////////////////////

// valueCallback is a utility struct that holds a callback function and additional information that are required to
// ensure the correct execution order of callbacks in the Value.
type valueCallback[T comparable] struct {
	// id is the unique identifier of the callback.
	id UniqueID

	// callback is the function that is executed when the callback is triggered.
	callback func(prevValue, newValue T)

	// lastTriggerID is the last triggerID that was used to trigger the callback.
	lastTriggerID int

	// triggerMutex is used to ensure that the callback is only triggered once per triggerID.
	triggerMutex sync.Mutex
}

// newValueCallback creates a new valueCallback instance.
func newValueCallback[T comparable](id UniqueID, callbackFunc func(prevValue, newValue T), updateIndex int) *valueCallback[T] {
	return &valueCallback[T]{
		id:            id,
		callback:      callbackFunc,
		lastTriggerID: updateIndex,
	}
}

// trigger triggers the callback if the triggerID is different from the last triggerID.
func (c *valueCallback[T]) trigger(triggerID int, prevValue, newValue T) {
	c.triggerMutex.Lock()
	defer c.triggerMutex.Unlock()

	if triggerID != c.lastTriggerID {
		c.lastTriggerID = triggerID

		c.callback(prevValue, newValue)
	}
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////
