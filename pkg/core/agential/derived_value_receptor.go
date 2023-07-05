package agential

import (
	"sync"

	"github.com/iotaledger/hive.go/lo"
)

// DerivedValueReceptor is a ValueReceptor that derives its value from other input values.
type DerivedValueReceptor[ValueType comparable] struct {
	// ValueReceptor is the ValueReceptor that holds the output value of the DerivedValueReceptor.
	*ValueReceptor[ValueType]

	// unsubscribe is the function that is used to unsubscribe the DerivedValueReceptor from the inputs.
	unsubscribe func()

	// unsubscribeMutex is the mutex that is used to synchronize access to the unsubscribe function.
	unsubscribeMutex sync.Mutex
}

// Unsubscribe unsubscribes the DerivedValueReceptor from its input values.
func (d *DerivedValueReceptor[ValueType]) Unsubscribe() {
	d.unsubscribeMutex.Lock()
	defer d.unsubscribeMutex.Unlock()

	if d.unsubscribe != nil {
		d.unsubscribe()
	}

	d.unsubscribe = nil
}

// DeriveValueReceptorFrom1Input creates a DerivedValueReceptor that transforms the value of one input value receptor into a different one.
func DeriveValueReceptorFrom1Input[ValueType, InputType1 comparable, InputValueType1 Value[InputType1]](containingAgent Agent, transformationFunc func(InputType1) ValueType, input1 *InputValueType1) *DerivedValueReceptor[ValueType] {
	d := &DerivedValueReceptor[ValueType]{
		ValueReceptor: NewValueReceptor[ValueType](),
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

// DeriveValueReceptorFrom2Inputs creates a DerivedValueReceptor that transforms the value of two input value receptors into a different one.
func DeriveValueReceptorFrom2Inputs[OutputType, InputType1, InputType2 comparable, InputValueType1 Value[InputType1], InputValueType2 Value[InputType2]](containingAgent Agent, transitionFunc func(InputType1, InputType2) OutputType, input1 *InputValueType1, input2 *InputValueType2) *DerivedValueReceptor[OutputType] {
	d := &DerivedValueReceptor[OutputType]{
		ValueReceptor: NewValueReceptor[OutputType](),
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

// DeriveValueFrom3Inputs creates a DerivedValueReceptor that transforms the value of three input ValueReceptors into a different one.
func DeriveValueFrom3Inputs[OutputType, InputType1, InputType2, InputType3 comparable, InputValueType1 Value[InputType1], InputValueType2 Value[InputType2], InputValueType3 Value[InputType3]](containingAgent Agent, transitionFunc func(InputType1, InputType2, InputType3) OutputType, input1 *InputValueType1, input2 *InputValueType2, input3 *InputValueType3) *DerivedValueReceptor[OutputType] {
	d := &DerivedValueReceptor[OutputType]{
		ValueReceptor: NewValueReceptor[OutputType](),
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
