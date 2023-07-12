package reactive

import (
	"sync"

	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/core/types"
)

// region Variable /////////////////////////////////////////////////////////////////////////////////////////////////////

// Variable represents a variable that can be read and written and that informs subscribed consumers about updates.
type Variable[Type comparable] interface {
	// Set sets the new value and triggers the registered callbacks if the value has changed.
	Set(newValue Type) (previousValue Type)

	// Compute sets the new value by applying the given function to the current value and triggers the registered
	// callbacks if the value has changed.
	Compute(computeFunc func(currentValue Type) Type) (previousValue Type)

	// ReadableVariable imports the interface that allows subscribers to read the value and to be notified when it changes.
	ReadableVariable[Type]
}

// NewVariable creates a new Variable instance with an optional transformation function that can be used to rewrite the
// value before it is stored.
func NewVariable[Type comparable](transformationFunc ...func(currentValue Type, newValue Type) Type) Variable[Type] {
	return newVariable(transformationFunc...)
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region ReadableVariable /////////////////////////////////////////////////////////////////////////////////////////////

// ReadableVariable represents a variable that can be read and that informs subscribed consumers about updates.
type ReadableVariable[Type comparable] interface {
	// Get returns the current value.
	Get() Type

	// OnUpdate registers the given callback that is triggered when the value changes.
	OnUpdate(consumer func(oldValue, newValue Type), triggerWithInitialZeroValue ...bool) (unsubscribe func())
}

// NewReadableVariable creates a new ReadableVariable instance with the given value.
func NewReadableVariable[Type comparable](value Type) ReadableVariable[Type] {
	return newReadableVariable(value)
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region DerivedVariable //////////////////////////////////////////////////////////////////////////////////////////////

// DerivedVariable is a Variable that automatically derives its value from other input values.
type DerivedVariable[Type comparable] interface {
	// Variable is the variable that holds the derived value.
	Variable[Type]

	// Unsubscribe unsubscribes the DerivedVariable from its input values.
	Unsubscribe()
}

// DeriveVariableFromInput creates a DerivedVariable that transforms an input value into a different one.
func DeriveVariableFromInput[Type, InputType1 comparable, InputValueType1 ReadableVariable[InputType1]](compute func(InputType1) Type, input1 *InputValueType1, lazyInitEvent ...Event) DerivedVariable[Type] {
	return newDerivedVariable[Type](func(d DerivedVariable[Type]) func() {
		return (*input1).OnUpdate(func(_, input1 InputType1) {
			d.Compute(func(_ Type) Type { return compute(input1) })
		}, true)
	}, lo.First(lazyInitEvent))
}

// DeriveVariableFrom2Inputs creates a DerivedVariable that transforms two input values into a different one.
func DeriveVariableFrom2Inputs[Type, InputType1, InputType2 comparable, InputValueType1 ReadableVariable[InputType1], InputValueType2 ReadableVariable[InputType2]](compute func(InputType1, InputType2) Type, input1 *InputValueType1, input2 *InputValueType2, lazyInitEvent ...Event) DerivedVariable[Type] {
	return newDerivedVariable[Type](func(d DerivedVariable[Type]) func() {
		return lo.Batch(
			(*input1).OnUpdate(func(_, input1 InputType1) {
				d.Compute(func(_ Type) Type { return compute(input1, (*input2).Get()) })
			}, true),

			(*input2).OnUpdate(func(_, input2 InputType2) {
				d.Compute(func(_ Type) Type { return compute((*input1).Get(), input2) })
			}, true),
		)
	}, lo.First(lazyInitEvent))
}

// DeriveVariableFrom3Inputs creates a DerivedVariable that transforms three input values into a different one.
func DeriveVariableFrom3Inputs[Type, InputType1, InputType2, InputType3 comparable, InputValueType1 ReadableVariable[InputType1], InputValueType2 ReadableVariable[InputType2], InputValueType3 ReadableVariable[InputType3]](compute func(InputType1, InputType2, InputType3) Type, input1 *InputValueType1, input2 *InputValueType2, input3 *InputValueType3, lazyInitEvent ...Event) DerivedVariable[Type] {
	return newDerivedVariable[Type](func(d DerivedVariable[Type]) func() {
		return lo.Batch(
			(*input1).OnUpdate(func(_, input1 InputType1) {
				d.Compute(func(_ Type) Type { return compute(input1, (*input2).Get(), (*input3).Get()) })
			}, true),

			(*input2).OnUpdate(func(_, input2 InputType2) {
				d.Compute(func(_ Type) Type { return compute((*input1).Get(), input2, (*input3).Get()) })
			}, true),

			(*input3).OnUpdate(func(_, input3 InputType3) {
				d.Compute(func(_ Type) Type { return compute((*input1).Get(), (*input2).Get(), input3) })
			}, true),
		)
	}, lo.First(lazyInitEvent))
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region variable /////////////////////////////////////////////////////////////////////////////////////////////////////

// variable is the default implementation of the Variable interface.
type variable[Type comparable] struct {
	*readableVariable[Type]

	// transformationFunc is the function that is used to transform the value before it is stored.
	transformationFunc func(currentValue Type, newValue Type) Type

	// mutex is used to make the Set and Compute methods atomic.
	mutex sync.Mutex
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
	v.mutex.Lock()
	defer v.mutex.Unlock()

	newValue, previousValue, updateID, registeredCallbacks := v.updateValue(computeFunc)

	for _, registeredCallback := range registeredCallbacks {
		if registeredCallback.LockExecution(updateID) {
			registeredCallback.Invoke(previousValue, newValue)
			registeredCallback.UnlockExecution()
		}
	}

	return previousValue
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region readableVariable /////////////////////////////////////////////////////////////////////////////////////////////

// readableVariable is the default implementation of the ReadableVariable interface.
type readableVariable[Type comparable] struct {
	// value holds the current value.
	value Type

	// registeredCallbacks holds the callbacks that are triggered when the value changes.
	registeredCallbacks *shrinkingmap.ShrinkingMap[types.UniqueID, *callback[func(prevValue, newValue Type)]]

	// uniqueUpdateID is used to derive a unique identifier for each update.
	uniqueUpdateID types.UniqueID

	// uniqueCallbackID is used to derive a unique identifier for each callback.
	uniqueCallbackID types.UniqueID

	// mutex is used to ensure that updating the value and registering/unregistering callbacks is thread safe.
	mutex sync.RWMutex
}

// newReadableVariable creates a new readableVariable instance with an optional initial value.
func newReadableVariable[Type comparable](initialValue ...Type) *readableVariable[Type] {
	return &readableVariable[Type]{
		value:               lo.First(initialValue),
		registeredCallbacks: shrinkingmap.New[types.UniqueID, *callback[func(prevValue, newValue Type)]](),
	}
}

// Get returns the current value.
func (r *readableVariable[Type]) Get() Type {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	return r.value
}

// OnUpdate registers the given callback that is triggered when the value changes.
func (r *readableVariable[Type]) OnUpdate(callback func(prevValue, newValue Type), triggerWithInitialZeroValue ...bool) (unsubscribe func()) {
	r.mutex.Lock()

	currentValue := r.value
	createdCallback := newCallback[func(prevValue, newValue Type)](r.uniqueCallbackID.Next(), callback)
	r.registeredCallbacks.Set(createdCallback.ID, createdCallback)

	// grab the execution lock before we unlock the mutex, so the callback cannot be triggered by another
	// thread updating the value before we have called the callback with the initial value
	createdCallback.LockExecution(r.uniqueUpdateID)
	defer createdCallback.UnlockExecution()

	r.mutex.Unlock()

	var emptyValue Type
	if currentValue != emptyValue || lo.First(triggerWithInitialZeroValue) {
		createdCallback.Invoke(emptyValue, currentValue)
	}

	return func() {
		r.registeredCallbacks.Delete(createdCallback.ID)

		createdCallback.MarkUnsubscribed()
	}
}

// updateValue atomically prepares the trigger by setting the new value and returning the new value, the
// previous value, the triggerID and the callbacks to trigger.
func (r *readableVariable[Type]) updateValue(newValueGenerator func(Type) Type) (newValue, previousValue Type, triggerID types.UniqueID, callbacksToTrigger []*callback[func(prevValue, newValue Type)]) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if previousValue, newValue = r.value, newValueGenerator(previousValue); newValue == previousValue {
		return newValue, previousValue, 0, nil
	}

	r.value = newValue

	return newValue, previousValue, r.uniqueUpdateID.Next(), r.registeredCallbacks.Values()
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region derivedVariable //////////////////////////////////////////////////////////////////////////////////////////////

// derivedVariable implements the DerivedVariable interface.
type derivedVariable[ValueType comparable] struct {
	// Variable is the Variable that holds the derived value.
	Variable[ValueType]

	// unsubscribe is the function that is used to unsubscribe the derivedVariable from the inputs.
	unsubscribe func()

	// unsubscribeMutex is the mutex that is used to synchronize access to the unsubscribe function.
	unsubscribeMutex sync.Mutex
}

// newDerivedVariable creates a new derivedVariable instance.
func newDerivedVariable[ValueType comparable](subscribe func(DerivedVariable[ValueType]) func(), lazyInitEvent Event) *derivedVariable[ValueType] {
	d := &derivedVariable[ValueType]{
		Variable: NewVariable[ValueType](),
	}

	if lazyInitEvent == nil {
		d.unsubscribe = subscribe(d)
	} else {
		d.unsubscribe = lazyInitEvent.OnTrigger(func() {
			d.unsubscribeMutex.Lock()
			defer d.unsubscribeMutex.Unlock()

			if d.unsubscribe == nil {
				return
			}

			d.unsubscribe = subscribe(d)
		})
	}

	return d
}

// Unsubscribe unsubscribes the DerivedVariable from its input values.
func (d *derivedVariable[ValueType]) Unsubscribe() {
	d.unsubscribeMutex.Lock()
	defer d.unsubscribeMutex.Unlock()

	if d.unsubscribe != nil {
		d.unsubscribe()
		d.unsubscribe = nil
	}
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////
