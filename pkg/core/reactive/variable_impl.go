package reactive

import (
	"sync"

	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
)

// region variable /////////////////////////////////////////////////////////////////////////////////////////////////////

// variable is the default implementation of the Variable interface.
type variable[Type comparable] struct {
	*readableVariable[Type]

	// transformationFunc is the function that is used to transform the value before it is stored.
	transformationFunc func(currentValue Type, newValue Type) Type

	// updateOrderMutex is used to make sure that write operations are executed sequentially (all subscribers are
	// notified before the next write operation is executed).
	updateOrderMutex sync.Mutex
}

// newVariable creates a new variable with an optional transformation function that can be used to rewrite the value
// before it is stored.
func newVariable[Type comparable](transformationFunc ...func(currentValue Type, newValue Type) Type) *variable[Type] {
	return &variable[Type]{
		transformationFunc: lo.First(transformationFunc, func(_ Type, newValue Type) Type { return newValue }),
		readableVariable:   newReadableVariable[Type](),
	}
}

// Set sets the new value and triggers the registered callbacks if the value has changed.
func (v *variable[Type]) Set(newValue Type) (previousValue Type) {
	return v.Compute(func(Type) Type { return newValue })
}

// Compute computes the new value based on the current value and triggers the registered callbacks if the value changed.
func (v *variable[Type]) Compute(computeFunc func(currentValue Type) Type) (previousValue Type) {
	v.updateOrderMutex.Lock()
	defer v.updateOrderMutex.Unlock()

	newValue, previousValue, updateID, registeredCallbacks := v.updateValue(computeFunc)

	for _, registeredCallback := range registeredCallbacks {
		if registeredCallback.LockExecution(updateID) {
			registeredCallback.Invoke(previousValue, newValue)
			registeredCallback.UnlockExecution()
		}
	}

	return previousValue
}

// updateValue atomically prepares the trigger by setting the new value and returning the new value, the previous value,
// the triggerID and the callbacks to trigger.
func (v *variable[Type]) updateValue(newValueGenerator func(Type) Type) (newValue, previousValue Type, triggerID uniqueID, callbacksToTrigger []*callback[func(prevValue, newValue Type)]) {
	v.valueMutex.Lock()
	defer v.valueMutex.Unlock()

	if previousValue, newValue = v.value, newValueGenerator(previousValue); newValue == previousValue {
		return newValue, previousValue, 0, nil
	}

	v.value = newValue

	return newValue, previousValue, v.uniqueUpdateID.Next(), v.registeredCallbacks.Values()
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region readableVariable /////////////////////////////////////////////////////////////////////////////////////////////

// readableVariable is the default implementation of the ReadableVariable interface.
type readableVariable[Type comparable] struct {
	// value holds the current value.
	value Type

	// registeredCallbacks holds the callbacks that are triggered when the value changes.
	registeredCallbacks *shrinkingmap.ShrinkingMap[uniqueID, *callback[func(prevValue, newValue Type)]]

	// uniqueUpdateID is used to derive a unique identifier for each update.
	uniqueUpdateID uniqueID

	// uniqueCallbackID is used to derive a unique identifier for each callback.
	uniqueCallbackID uniqueID

	// valueMutex is used to ensure that access to the value is synchronized.
	valueMutex sync.RWMutex
}

// newReadableVariable creates a new readableVariable instance with an optional initial value.
func newReadableVariable[Type comparable](initialValue ...Type) *readableVariable[Type] {
	return &readableVariable[Type]{
		value:               lo.First(initialValue),
		registeredCallbacks: shrinkingmap.New[uniqueID, *callback[func(prevValue, newValue Type)]](),
	}
}

// Get returns the current value.
func (r *readableVariable[Type]) Get() Type {
	r.valueMutex.RLock()
	defer r.valueMutex.RUnlock()

	return r.value
}

// OnUpdate registers the given callback that is triggered when the value changes.
func (r *readableVariable[Type]) OnUpdate(callback func(prevValue, newValue Type), triggerWithInitialZeroValue ...bool) (unsubscribe func()) {
	r.valueMutex.Lock()

	currentValue := r.value
	createdCallback := newCallback[func(prevValue, newValue Type)](r.uniqueCallbackID.Next(), callback)
	r.registeredCallbacks.Set(createdCallback.ID, createdCallback)

	// grab the execution lock before we unlock the mutex, so the callback cannot be triggered by another
	// thread updating the value before we have called the callback with the initial value
	createdCallback.LockExecution(r.uniqueUpdateID)
	defer createdCallback.UnlockExecution()

	r.valueMutex.Unlock()

	var emptyValue Type
	if currentValue != emptyValue || lo.First(triggerWithInitialZeroValue) {
		createdCallback.Invoke(emptyValue, currentValue)
	}

	return func() {
		r.registeredCallbacks.Delete(createdCallback.ID)

		createdCallback.MarkUnsubscribed()
	}
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region derivedVariable //////////////////////////////////////////////////////////////////////////////////////////////

// derivedVariable implements the DerivedVariable interface.
type derivedVariable[ValueType comparable] struct {
	// Variable is the Variable that holds the derived value.
	Variable[ValueType]

	// unsubscribe is the function that is used to unsubscribe the derivedVariable from the inputs.
	unsubscribe func()

	// unsubscribeOnce is used to make sure that the unsubscribe function is only called once.
	unsubscribeOnce sync.Once
}

// newDerivedVariable creates a new derivedVariable instance.
func newDerivedVariable[ValueType comparable](subscribe func(DerivedVariable[ValueType]) func()) *derivedVariable[ValueType] {
	d := &derivedVariable[ValueType]{
		Variable: NewVariable[ValueType](),
	}

	d.unsubscribe = subscribe(d)

	return d
}

// Unsubscribe unsubscribes the DerivedVariable from its input values.
func (d *derivedVariable[ValueType]) Unsubscribe() {
	d.unsubscribeOnce.Do(d.unsubscribe)
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////
