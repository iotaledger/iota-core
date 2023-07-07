package reactive

import (
	"sync"

	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/core/types"
)

// Variable is a reactive component that acts as a thread-safe variable that informs subscribed consumers about updates.
type Variable[T comparable] interface {
	// Set sets the new value and triggers the registered callbacks if the value has changed.
	Set(newValue T) (previousValue T)

	// Compute sets the new value by applying the given function to the current value and triggers the registered
	// callbacks if the value has changed.
	Compute(computeFunc func(currentValue T) T) (previousValue T)

	// Value imports the interface that allows subscribers to read the value and to be notified when it changes.
	Value[T]
}

// NewVariable creates a new Variable instance with an optional transformation function that can be used to rewrite the
// set value before it is stored.
func NewVariable[T comparable](transformationFunc ...func(currentValue T, newValue T) T) Variable[T] {
	return &variable[T]{
		transformationFunc:  lo.First(transformationFunc, func(_ T, newValue T) T { return newValue }),
		registeredCallbacks: shrinkingmap.New[types.UniqueID, *callback[func(prevValue, newValue T)]](),
	}
}

// variable is the default implementation of the Variable interface.
type variable[T comparable] struct {
	// value holds the current value.
	value T

	// transformationFunc is the function that is used to transform the value before it is stored.
	transformationFunc func(currentValue T, newValue T) T

	// registeredCallbacks holds the callbacks that are triggered when the value changes.
	registeredCallbacks *shrinkingmap.ShrinkingMap[types.UniqueID, *callback[func(prevValue, newValue T)]]

	// uniqueUpdateID is used to derive a unique identifier for each update.
	uniqueUpdateID types.UniqueID

	// uniqueCallbackID is used to derive a unique identifier for each callback.
	uniqueCallbackID types.UniqueID

	// mutex is used to ensure that updating the value and registering/unregistering callbacks is thread safe.
	mutex sync.RWMutex

	// setMutex is used to ensure that the order of updates is preserved.
	setMutex sync.Mutex
}

// Get returns the current value.
func (v *variable[T]) Get() T {
	v.mutex.RLock()
	defer v.mutex.RUnlock()

	return v.value
}

// Set sets the new value and triggers the registered callbacks if the value has changed.
func (v *variable[T]) Set(newValue T) (previousValue T) {
	return v.Compute(func(T) T { return newValue })
}

// Compute computes the new value based on the current value and triggers the registered callbacks if the value changed.
func (v *variable[T]) Compute(computeFunc func(currentValue T) T) (previousValue T) {
	v.setMutex.Lock()
	defer v.setMutex.Unlock()

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
func (v *variable[T]) OnUpdate(callback func(prevValue, newValue T), triggerWithInitialZeroValue ...bool) (unsubscribe func()) {
	v.mutex.Lock()

	currentValue := v.value

	createdCallback := newCallback[func(prevValue, newValue T)](v.uniqueCallbackID.Next(), callback)
	v.registeredCallbacks.Set(createdCallback.ID, createdCallback)

	// we intertwine the mutexes to ensure that the callback is guaranteed to be triggered with the current value from
	// here first even if the value is updated in parallel.
	createdCallback.Lock(v.uniqueUpdateID)
	defer createdCallback.Unlock()

	v.mutex.Unlock()

	var emptyValue T
	if currentValue != emptyValue || lo.First(triggerWithInitialZeroValue) {
		createdCallback.Invoke(emptyValue, currentValue)
	}

	return func() {
		v.registeredCallbacks.Delete(createdCallback.ID)

		createdCallback.MarkUnsubscribed()
	}
}

// prepareDynamicTrigger atomically prepares the trigger by setting the new value and returning the new value, the
// previous value, the triggerID and the callbacks to trigger.
func (v *variable[T]) prepareDynamicTrigger(newValueGenerator func(T) T) (newValue, previousValue T, triggerID types.UniqueID, callbacksToTrigger []*callback[func(prevValue, newValue T)]) {
	v.mutex.Lock()
	defer v.mutex.Unlock()

	if previousValue, newValue = v.value, newValueGenerator(previousValue); newValue == previousValue {
		return newValue, previousValue, 0, nil
	}

	v.value = newValue

	return newValue, previousValue, v.uniqueUpdateID.Next(), v.registeredCallbacks.Values()
}
