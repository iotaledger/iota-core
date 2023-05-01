package promise

import (
	"sync"

	"github.com/iotaledger/hive.go/ds/orderedmap"
	"github.com/iotaledger/hive.go/lo"
)

// Value is a value that can be updated and notifies registered updateCallbacks about updates.
type Value[T comparable] struct {
	// value is the current value.
	value T

	// zeroValue is the zero value of the value type.
	zeroValue T

	// updateCallbacks are called when the value is updated.
	updateCallbacks *orderedmap.OrderedMap[CallbackID, func(T)]

	// valueMutex is used to synchronize access to the value.
	valueMutex sync.RWMutex

	// updateCallbacksMutex is used to synchronize access to the updateCallbacks.
	updateCallbacksMutex sync.RWMutex

	// updateOrderMutex is used to ensure that updateCallbacks get informed about updates in the order they were triggered.
	updateOrderMutex sync.Mutex
}

// NewValue creates a new Value.
func NewValue[T comparable](optZeroValue ...T) *Value[T] {
	return &Value[T]{
		value:           lo.First(optZeroValue),
		zeroValue:       lo.First(optZeroValue),
		updateCallbacks: orderedmap.New[CallbackID, func(T)](),
	}
}

// Get returns the current value.
func (v *Value[T]) Get() T {
	v.valueMutex.RLock()
	defer v.valueMutex.RUnlock()

	return v.value
}

// Set updates the current value and calls the registered updateCallbacks.
func (v *Value[T]) Set(value T) (previousValue T) {
	setValue := func(value T) (previousValue T) {
		v.valueMutex.Lock()
		defer v.valueMutex.Unlock()

		if previousValue = v.value; previousValue != value {
			v.value = value
		}

		return previousValue
	}

	v.updateCallbacksMutex.RLock()
	defer v.updateCallbacksMutex.RUnlock()

	v.updateOrderMutex.Lock()
	defer v.updateOrderMutex.Unlock()

	if previousValue = setValue(value); previousValue != value {
		v.updateCallbacks.ForEach(func(_ CallbackID, callback func(value T)) bool {
			callback(value)
			return true
		})
	}

	return previousValue
}

// OnUpdate registers a callback that gets called when the value is updated. The callback is called immediately with the
// current value if it is not the zero value.
func (v *Value[T]) OnUpdate(callback func(value T)) (unsubscribe func()) {
	v.updateCallbacksMutex.Lock()
	defer v.updateCallbacksMutex.Unlock()

	callbackID := NewCallbackID()
	v.updateCallbacks.Set(callbackID, callback)

	if v.value != v.zeroValue {
		callback(v.value)
	}

	return func() {
		v.updateCallbacksMutex.Lock()
		defer v.updateCallbacksMutex.Unlock()

		v.updateCallbacks.Delete(callbackID)
	}
}
