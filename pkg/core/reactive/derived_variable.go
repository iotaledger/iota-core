package reactive

import (
	"sync"

	"github.com/iotaledger/hive.go/lo"
)

// DerivedVariable is a Variable that automatically derives its value from other input values.
type DerivedVariable[Type comparable] interface {
	// Variable is the variable that holds the derived value.
	Variable[Type]

	// Unsubscribe unsubscribes the DerivedVariable from its input values.
	Unsubscribe()
}

// DeriveVariableFromValue creates a DerivedVariable that transforms an input value into a different one.
func DeriveVariableFromValue[Type, InputType1 comparable, InputValueType1 Value[InputType1]](compute func(InputType1) Type, input1 *InputValueType1, lazyInitEvent ...Event) DerivedVariable[Type] {
	return newDerivedVariable[Type](func(d DerivedVariable[Type]) func() {
		return (*input1).OnUpdate(func(_, input1 InputType1) {
			d.Compute(func(_ Type) Type { return compute(input1) })
		}, true)
	}, lo.First(lazyInitEvent))
}

// DeriveVariableFrom2Values creates a DerivedVariable that transforms two input values into a different one.
func DeriveVariableFrom2Values[Type, InputType1, InputType2 comparable, InputValueType1 Value[InputType1], InputValueType2 Value[InputType2]](compute func(InputType1, InputType2) Type, input1 *InputValueType1, input2 *InputValueType2, lazyInitEvent ...Event) DerivedVariable[Type] {
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

// DeriveVariableFrom3Values creates a DerivedVariable that transforms three input values into a different one.
func DeriveVariableFrom3Values[Type, InputType1, InputType2, InputType3 comparable, InputValueType1 Value[InputType1], InputValueType2 Value[InputType2], InputValueType3 Value[InputType3]](compute func(InputType1, InputType2, InputType3) Type, input1 *InputValueType1, input2 *InputValueType2, input3 *InputValueType3, lazyInitEvent ...Event) DerivedVariable[Type] {
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
