package agential

import (
	"sync"

	"github.com/iotaledger/hive.go/lo"
)

// DerivedValueReceptor is a ValueReceptor that derives its value from other ValueReceptors.
type DerivedValueReceptor[OutputType comparable] struct {
	// ValueReceptor is the ValueReceptor that holds the output value of the DerivedValueReceptor.
	ValueReceptor[OutputType]

	// unsubscribe is the function that is used to unsubscribe the DerivedValueReceptor from the inputs.
	unsubscribe func()

	// unsubscribeMutex is the mutex that is used to synchronize access to the unsubscribe function.
	unsubscribeMutex sync.Mutex
}

// Unsubscribe unsubscribes the DerivedValueReceptor from the input value receptors.
func (d *DerivedValueReceptor[OutputType]) Unsubscribe() {
	d.unsubscribeMutex.Lock()
	defer d.unsubscribeMutex.Unlock()

	if d.unsubscribe != nil {
		d.unsubscribe()
	}

	d.unsubscribe = nil
}

// DeriveValueReceptorFrom1Input creates a DerivedValueReceptor that transforms the value of one input value receptor into a different one.
func DeriveValueReceptorFrom1Input[OutputType, InputType1 comparable, InputSource1 ValueReceptorReadOnly[InputType1]](containingAgent Agent, transformationFunc func(InputType1) OutputType, input1 *InputSource1) *DerivedValueReceptor[OutputType] {
	d := &DerivedValueReceptor[OutputType]{
		ValueReceptor: NewValueReceptor[OutputType](),
	}

	d.unsubscribe = containingAgent.Constructed().OnUpdate(func(_, _ bool) {
		d.unsubscribeMutex.Lock()
		defer d.unsubscribeMutex.Unlock()

		if d.unsubscribe == nil {
			return
		}

		d.unsubscribe = (*input1).OnUpdate(func(_, input1 InputType1) {
			d.Compute(func(_ OutputType) OutputType { return transformationFunc(input1) })
		})
	})

	return d
}

// DeriveValueReceptorFrom2Inputs creates a DerivedValueReceptor that transforms the value of two input value receptors into a different one.
func DeriveValueReceptorFrom2Inputs[OutputType, InputType1, InputType2 comparable, InputSource1 ValueReceptorReadOnly[InputType1], InputSource2 ValueReceptorReadOnly[InputType2]](agent Agent, transitionFunc func(InputType1, InputType2) OutputType, input1 *InputSource1, input2 *InputSource2) *DerivedValueReceptor[OutputType] {
	d := &DerivedValueReceptor[OutputType]{
		ValueReceptor: NewValueReceptor[OutputType](),
	}

	d.unsubscribe = agent.Constructed().OnUpdate(func(_, _ bool) {
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
func DeriveValueFrom3Inputs[OutputType, InputType1, InputType2, InputType3 comparable, InputSource1 ValueReceptorReadOnly[InputType1], InputSource2 ValueReceptorReadOnly[InputType2], InputSource3 ValueReceptorReadOnly[InputType3]](agent Agent, transitionFunc func(InputType1, InputType2, InputType3) OutputType, input1 *InputSource1, input2 *InputSource2, input3 *InputSource3) *DerivedValueReceptor[OutputType] {
	d := &DerivedValueReceptor[OutputType]{
		ValueReceptor: NewValueReceptor[OutputType](),
	}

	agent.Constructed().OnUpdate(func(_, _ bool) {
		(*input1).OnUpdate(func(_, input1 InputType1) {
			d.Compute(func(_ OutputType) OutputType { return transitionFunc(input1, (*input2).Get(), (*input3).Get()) })
		})

		(*input2).OnUpdate(func(_, input2 InputType2) {
			d.Compute(func(_ OutputType) OutputType { return transitionFunc((*input1).Get(), input2, (*input3).Get()) })
		})

		(*input3).OnUpdate(func(_, input3 InputType3) {
			d.Compute(func(_ OutputType) OutputType { return transitionFunc((*input1).Get(), (*input2).Get(), input3) })
		})
	})

	return d
}
