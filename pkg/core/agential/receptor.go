package agential

import (
	"sync"

	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/core/types"
)

type Receptor[T comparable] interface {
	Set(newValue T) (previousValue T)

	Compute(computeFunc func(currentValue T) T) (previousValue T)

	ReadOnlyReceptor[T]
}

type ReadOnlyReceptor[T comparable] interface {
	Get() T

	OnUpdate(consumer func(old, new T)) (unsubscribe func())
}

// NewReceptor creates a new Receptor instance.
func NewReceptor[T comparable](optUpdateFunc ...func(currentValue T, newValue T) T) Receptor[T] {
	return newReceptor(optUpdateFunc...)
}

// receptor is a thread safe value that allows multiple consumers to subscribe to changes by registering callbacks that are
// executed when the value is updated.
//
// The registered callbacks are guaranteed to receive all updates in exactly the same order as they happened, but
// there is no guarantee about the order of updates between different callbacks (they are executed in a random order).
//
// It is however guaranteed, that no callback will be ahead of other callbacks by more than 1 "round".
type receptor[T comparable] struct {
	// value is the current value.
	value T

	updateFunc func(currentValue T, newValue T) T

	// updateCallbacks are the registered callbacks that are triggered when the value changes.
	updateCallbacks *shrinkingmap.ShrinkingMap[types.UniqueID, *Callback[func(prevValue, newValue T)]]

	// uniqueUpdateID is the unique ID that is used to identify an update.
	uniqueUpdateID types.UniqueID

	// uniqueCallbackID is the unique ID that is used to identify a callback.
	uniqueCallbackID types.UniqueID

	// mutex is used to ensure that updating the value and registering/unregistering callbacks is thread safe.
	mutex sync.RWMutex

	// setOrderMutex is an additional mutex that is used to ensure that the order of updates is ensured.
	setOrderMutex sync.Mutex

	// optTriggerWithInitialZeroValue is an option that can be set to make the OnUpdate callbacks trigger immediately
	// on subscription even if the current value is the zero value.
	optTriggerWithInitialZeroValue bool
}

// newReceptor creates a new receptor instance.
func newReceptor[T comparable](optUpdateFunc ...func(currentValue T, newValue T) T) *receptor[T] {
	return &receptor[T]{
		updateFunc:      lo.First(optUpdateFunc, func(_ T, newValue T) T { return newValue }),
		updateCallbacks: shrinkingmap.New[types.UniqueID, *Callback[func(prevValue, newValue T)]](),
	}
}

// Get returns the current value.
func (r *receptor[T]) Get() T {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	return r.value
}

// Set sets the new value and triggers the registered callbacks if the value has changed.
func (r *receptor[T]) Set(newValue T) (previousValue T) {
	return r.Compute(func(T) T { return newValue })
}

// Compute computes the new value based on the current value and triggers the registered callbacks if the value changed.
func (r *receptor[T]) Compute(computeFunc func(currentValue T) T) (previousValue T) {
	r.setOrderMutex.Lock()
	defer r.setOrderMutex.Unlock()

	newValue, previousValue, updateID, callbacksToTrigger := r.prepareDynamicTrigger(computeFunc)
	for _, callback := range callbacksToTrigger {
		if callback.Lock(updateID) {
			callback.Invoke(previousValue, newValue)
			callback.Unlock()
		}
	}

	return previousValue
}

// OnUpdate registers a callback that is triggered when the value changes.
func (r *receptor[T]) OnUpdate(callback func(prevValue, newValue T)) (unsubscribe func()) {
	r.mutex.Lock()

	currentValue := r.value

	newCallback := NewCallback[func(prevValue, newValue T)](r.uniqueCallbackID.Next(), callback)
	r.updateCallbacks.Set(newCallback.ID, newCallback)

	// we intertwine the mutexes to ensure that the callback is guaranteed to be triggered with the current value from
	// here first even if the value is updated in parallel.
	newCallback.Lock(r.uniqueUpdateID)
	defer newCallback.Unlock()

	r.mutex.Unlock()

	var emptyValue T
	if r.optTriggerWithInitialZeroValue || currentValue != emptyValue {
		newCallback.Invoke(emptyValue, currentValue)
	}

	return func() {
		r.updateCallbacks.Delete(newCallback.ID)

		newCallback.MarkUnsubscribed()
	}
}

// prepareDynamicTrigger atomically prepares the trigger by setting the new value and returning the new value, the
// previous value, the triggerID and the callbacks to trigger.
func (r *receptor[T]) prepareDynamicTrigger(newValueGenerator func(T) T) (newValue, previousValue T, triggerID types.UniqueID, callbacksToTrigger []*Callback[func(prevValue, newValue T)]) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if previousValue, newValue = r.value, newValueGenerator(previousValue); newValue == previousValue {
		return newValue, previousValue, 0, nil
	}

	r.value = newValue

	return newValue, previousValue, r.uniqueUpdateID.Next(), r.updateCallbacks.Values()
}
