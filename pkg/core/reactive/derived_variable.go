package reactive

import (
	"sync"

	"github.com/iotaledger/hive.go/lo"
)

// DerivedVariable is a variable that derives its value from other input values.
type DerivedVariable[ValueType comparable] struct {
	// ValueReceptor is the variable that holds the output value of the DerivedVariable.
	Variable[ValueType]

	// unsubscribe is the function that is used to unsubscribe the DerivedVariable from the inputs.
	unsubscribe func()

	// unsubscribeMutex is the mutex that is used to synchronize access to the unsubscribe function.
	unsubscribeMutex sync.Mutex
}

// Unsubscribe unsubscribes the DerivedVariable from its input values.
func (d *DerivedVariable[ValueType]) Unsubscribe() {
	d.unsubscribeMutex.Lock()
	defer d.unsubscribeMutex.Unlock()

	if d.unsubscribe != nil {
		d.unsubscribe()
	}

	d.unsubscribe = nil
}

// NewDerivedVariable1 creates a DerivedVariable that transforms the value of one input value receptor into a different one.
func NewDerivedVariable1[ValueType, InputType1 comparable, InputValueType1 Value[InputType1]](containingAgent Agent, transformationFunc func(InputType1) ValueType, input1 *InputValueType1) *DerivedVariable[ValueType] {
	d := &DerivedVariable[ValueType]{
		Variable: NewVariable[ValueType](),
	}

	d.unsubscribe = containingAgent.Constructed().OnUpdate(func(_, _ bool) {
		d.unsubscribeMutex.Lock()
		defer d.unsubscribeMutex.Unlock()

		if d.unsubscribe == nil {
			return
		}

		d.unsubscribe = (*input1).OnUpdate(func(_, input1 InputType1) {
			d.Compute(func(_ ValueType) ValueType { return transformationFunc(input1) })
		})
	})

	return d
}

// NewDerivedVariable2 creates a DerivedVariable that transforms the value of two input value receptors into a different one.
func NewDerivedVariable2[OutputType, InputType1, InputType2 comparable, InputValueType1 Value[InputType1], InputValueType2 Value[InputType2]](containingAgent Agent, transitionFunc func(InputType1, InputType2) OutputType, input1 *InputValueType1, input2 *InputValueType2) *DerivedVariable[OutputType] {
	d := &DerivedVariable[OutputType]{
		Variable: NewVariable[OutputType](),
	}

	d.unsubscribe = containingAgent.Constructed().OnUpdate(func(_, _ bool) {
		d.unsubscribeMutex.Lock()
		defer d.unsubscribeMutex.Unlock()

		if d.unsubscribe == nil {
			return
		}

		d.unsubscribe = lo.Batch(
			(*input1).OnUpdate(func(_, input1 InputType1) {
				d.Compute(func(_ OutputType) OutputType { return transitionFunc(input1, (*input2).Get()) })
			}),

			(*input2).OnUpdate(func(_, input2 InputType2) {
				d.Compute(func(_ OutputType) OutputType { return transitionFunc((*input1).Get(), input2) })
			}),
		)
	})

	return d
}

// NewDerivedVariable3 creates a DerivedVariable that transforms the value of three input ValueReceptors into a different one.
func NewDerivedVariable3[OutputType, InputType1, InputType2, InputType3 comparable, InputValueType1 Value[InputType1], InputValueType2 Value[InputType2], InputValueType3 Value[InputType3]](containingAgent Agent, transitionFunc func(InputType1, InputType2, InputType3) OutputType, input1 *InputValueType1, input2 *InputValueType2, input3 *InputValueType3) *DerivedVariable[OutputType] {
	d := &DerivedVariable[OutputType]{
		Variable: NewVariable[OutputType](),
	}

	d.unsubscribe = containingAgent.Constructed().OnUpdate(func(_, _ bool) {
		d.unsubscribeMutex.Lock()
		defer d.unsubscribeMutex.Unlock()

		if d.unsubscribe == nil {
			return
		}

		d.unsubscribe = lo.Batch(
			(*input1).OnUpdate(func(_, input1 InputType1) {
				d.Compute(func(_ OutputType) OutputType { return transitionFunc(input1, (*input2).Get(), (*input3).Get()) })
			}),

			(*input2).OnUpdate(func(_, input2 InputType2) {
				d.Compute(func(_ OutputType) OutputType { return transitionFunc((*input1).Get(), input2, (*input3).Get()) })
			}),

			(*input3).OnUpdate(func(_, input3 InputType3) {
				d.Compute(func(_ OutputType) OutputType { return transitionFunc((*input1).Get(), (*input2).Get(), input3) })
			}),
		)
	})

	return d
}
