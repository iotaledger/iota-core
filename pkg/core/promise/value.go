package promise

import (
	"sync"

	"github.com/iotaledger/hive.go/ds/shrinkingmap"
)

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
	updateCallbacks *shrinkingmap.ShrinkingMap[UniqueID, *Callback[func(prevValue, newValue T)]]

	// uniqueUpdateID is the unique ID that is used to identify an update.
	uniqueUpdateID UniqueID

	// uniqueCallbackID is the unique ID that is used to identify a callback.
	uniqueCallbackID UniqueID

	// mutex is used to ensure that updating the value and registering/unregistering callbacks is thread safe.
	mutex sync.RWMutex

	// setOrderMutex is an additional mutex that is used to ensure that the order of updates is ensured.
	setOrderMutex sync.Mutex

	// optTriggerWithInitialZeroValue is an option that can be set to make the OnUpdate callbacks trigger immediately
	// on subscription even if the current value is the zero value.
	optTriggerWithInitialZeroValue bool
}

// NewValue creates a new Value instance.
func NewValue[T comparable]() *Value[T] {
	return &Value[T]{
		updateCallbacks: shrinkingmap.New[UniqueID, *Callback[func(prevValue, newValue T)]](),
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

	newValue, previousValue, updateID, callbacksToTrigger := v.prepareDynamicTrigger(computeFunc)
	for _, callback := range callbacksToTrigger {
		if callback.Lock(updateID) {
			callback.Invoke(previousValue, newValue)
			callback.Unlock()
		}
	}

	return previousValue
}

// OnUpdate registers a callback that is triggered when the value changes.
func (v *Value[T]) OnUpdate(callback func(prevValue, newValue T)) (unsubscribe func()) {
	v.mutex.Lock()

	currentValue := v.value

	newCallback := NewCallback[func(prevValue, newValue T)](v.uniqueCallbackID.Next(), callback)
	v.updateCallbacks.Set(newCallback.ID, newCallback)

	// we intertwine the mutexes to ensure that the callback is guaranteed to be triggered with the current value from
	// here first even if the value is updated in parallel.
	newCallback.Lock(v.uniqueUpdateID)
	defer newCallback.Unlock()

	v.mutex.Unlock()

	var emptyValue T
	if v.optTriggerWithInitialZeroValue || currentValue != emptyValue {
		newCallback.Invoke(emptyValue, currentValue)
	}

	return func() {
		v.updateCallbacks.Delete(newCallback.ID)

		newCallback.MarkUnsubscribed()
	}
}

// WithTriggerWithInitialZeroValue is an option that can be set to make the OnUpdate callbacks trigger immediately on
// subscription even if the current value is empty.
func (v *Value[T]) WithTriggerWithInitialZeroValue(trigger bool) *Value[T] {
	v.optTriggerWithInitialZeroValue = trigger

	return v
}

// prepareDynamicTrigger atomically prepares the trigger by setting the new value and returning the new value, the
// previous value, the triggerID and the callbacks to trigger.
func (v *Value[T]) prepareDynamicTrigger(newValueGenerator func(T) T) (newValue, previousValue T, triggerID UniqueID, callbacksToTrigger []*Callback[func(prevValue, newValue T)]) {
	v.mutex.Lock()
	defer v.mutex.Unlock()

	if previousValue, newValue = v.value, newValueGenerator(previousValue); newValue == previousValue {
		return newValue, previousValue, 0, nil
	}

	v.value = newValue

	return newValue, previousValue, v.uniqueUpdateID.Next(), v.updateCallbacks.Values()
}
