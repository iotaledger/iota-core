package reactive

import (
	"sync"

	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/core/types"
)

// Variable defines an interface for a reactive component that acts as a thread-safe variable that informs subscribed
// consumers about updates.
type Variable[Type comparable] interface {
	// Set sets the new value and triggers the registered callbacks if the value has changed.
	Set(newValue Type) (previousValue Type)

	// Compute sets the new value by applying the given function to the current value and triggers the registered
	// callbacks if the value has changed.
	Compute(computeFunc func(currentValue Type) Type) (previousValue Type)

	// Value imports the interface that allows subscribers to read the value and to be notified when it changes.
	Value[Type]
}

// NewVariable creates a new Variable instance with an optional transformation function that can be used to rewrite the
// set value before it is stored.
func NewVariable[Type comparable](transformationFunc ...func(currentValue Type, newValue Type) Type) Variable[Type] {
	return &variable[Type]{
		transformationFunc:  lo.First(transformationFunc, func(_ Type, newValue Type) Type { return newValue }),
		registeredCallbacks: shrinkingmap.New[types.UniqueID, *callback[func(prevValue, newValue Type)]](),
	}
}

// variable is the default implementation of the Variable interface.
type variable[Type comparable] struct {
	// value holds the current value.
	value Type

	// transformationFunc is the function that is used to transform the value before it is stored.
	transformationFunc func(currentValue Type, newValue Type) Type

	// registeredCallbacks holds the callbacks that are triggered when the value changes.
	registeredCallbacks *shrinkingmap.ShrinkingMap[types.UniqueID, *callback[func(prevValue, newValue Type)]]

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
func (v *variable[Type]) Get() Type {
	v.mutex.RLock()
	defer v.mutex.RUnlock()

	return v.value
}

// Set sets the new value and triggers the registered callbacks if the value has changed.
func (v *variable[Type]) Set(newValue Type) (previousValue Type) {
	return v.Compute(func(Type) Type { return newValue })
}

// Compute computes the new value based on the current value and triggers the registered callbacks if the value changed.
func (v *variable[Type]) Compute(computeFunc func(currentValue Type) Type) (previousValue Type) {
	v.setMutex.Lock()
	defer v.setMutex.Unlock()

	newValue, previousValue, updateID, callbacksToTrigger := v.prepareDynamicTrigger(computeFunc)
	for _, callback := range callbacksToTrigger {
		if callback.LockExecution(updateID) {
			callback.Invoke(previousValue, newValue)
			callback.UnlockExecution()
		}
	}

	return previousValue
}

// OnUpdate registers a callback that is triggered when the value changes.
func (v *variable[Type]) OnUpdate(callback func(prevValue, newValue Type), triggerWithInitialZeroValue ...bool) (unsubscribe func()) {
	v.mutex.Lock()

	currentValue := v.value

	createdCallback := newCallback[func(prevValue, newValue Type)](v.uniqueCallbackID.Next(), callback)
	v.registeredCallbacks.Set(createdCallback.ID, createdCallback)

	// grab the lock to make sure that the callback is not executed before we have called it with the initial value.
	createdCallback.LockExecution(v.uniqueUpdateID)
	defer createdCallback.UnlockExecution()

	v.mutex.Unlock()

	var emptyValue Type
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
func (v *variable[Type]) prepareDynamicTrigger(newValueGenerator func(Type) Type) (newValue, previousValue Type, triggerID types.UniqueID, callbacksToTrigger []*callback[func(prevValue, newValue Type)]) {
	v.mutex.Lock()
	defer v.mutex.Unlock()

	if previousValue, newValue = v.value, newValueGenerator(previousValue); newValue == previousValue {
		return newValue, previousValue, 0, nil
	}

	v.value = newValue

	return newValue, previousValue, v.uniqueUpdateID.Next(), v.registeredCallbacks.Values()
}
