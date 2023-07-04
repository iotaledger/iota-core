package agential

import "github.com/iotaledger/hive.go/lo"

func NewValueReceptorFromInput[TargetType, InputType1 comparable, InputSource1 ReadOnlyValueReceptor[InputType1]](agent Agent, transformationFunc func(InputType1) TargetType, input1 *InputSource1) ValueReceptor[TargetType] {
	t := newValueReceptor[TargetType]()

	agent.Constructed().OnUpdate(func(_, _ bool) {
		(*input1).OnUpdate(func(_, input1 InputType1) {
			t.Compute(func(_ TargetType) TargetType { return transformationFunc(input1) })
		})
	})

	return t
}

type ValueReceptorFrom2Inputs[TargetType, InputType1, InputType2 comparable] struct {
	*valueReceptor[TargetType]

	transitionFunc func(InputType1, InputType2) TargetType
}

func NewDerivedValueReceptorFrom2Inputs[TargetType, InputType1, InputType2 comparable, InputSource1 ReadOnlyValueReceptor[InputType1], InputSource2 ReadOnlyValueReceptor[InputType2]](agent Agent, transitionFunc func(InputType1, InputType2) TargetType, input1 *InputSource1, input2 *InputSource2) *ValueReceptorFrom2Inputs[TargetType, InputType1, InputType2] {
	t := &ValueReceptorFrom2Inputs[TargetType, InputType1, InputType2]{
		valueReceptor:  newValueReceptor[TargetType](),
		transitionFunc: transitionFunc,
	}

	agent.Constructed().OnUpdate(func(_, _ bool) {
		(*input1).OnUpdate(func(_, input1 InputType1) {
			t.Compute(func(_ TargetType) TargetType { return t.transitionFunc(input1, (*input2).Get()) })
		})

		(*input2).OnUpdate(func(_, input2 InputType2) {
			t.Compute(func(_ TargetType) TargetType { return t.transitionFunc((*input1).Get(), input2) })
		})
	})

	return t
}

type ValueTransformerWith3Inputs[TargetType, InputType1, InputType2, InputType3 comparable] struct {
	*valueReceptor[TargetType]

	transitionFunc func(InputType1, InputType2, InputType3) TargetType
}

func NewDerivedValueReceptorFrom3Inputs[TargetType, InputType1, InputType2, InputType3 comparable, InputSource1 ReadOnlyValueReceptor[InputType1], InputSource2 ReadOnlyValueReceptor[InputType2], InputSource3 ReadOnlyValueReceptor[InputType3]](agent Agent, transitionFunc func(InputType1, InputType2, InputType3) TargetType, input1 *InputSource1, input2 *InputSource2, input3 *InputSource3) *ValueTransformerWith3Inputs[TargetType, InputType1, InputType2, InputType3] {
	t := &ValueTransformerWith3Inputs[TargetType, InputType1, InputType2, InputType3]{
		valueReceptor:  newValueReceptor[TargetType](),
		transitionFunc: transitionFunc,
	}

	agent.Constructed().OnUpdate(func(_, _ bool) {
		(*input1).OnUpdate(func(_, input1 InputType1) {
			t.Compute(func(_ TargetType) TargetType { return t.transitionFunc(input1, (*input2).Get(), (*input3).Get()) })
		})

		(*input2).OnUpdate(func(_, input2 InputType2) {
			t.Compute(func(_ TargetType) TargetType { return t.transitionFunc((*input1).Get(), input2, (*input3).Get()) })
		})

		(*input3).OnUpdate(func(_, input3 InputType3) {
			t.Compute(func(_ TargetType) TargetType { return t.transitionFunc((*input1).Get(), (*input2).Get(), input3) })
		})
	})

	return t
}

type ThresholdTransformer[InputType comparable] struct {
	updateFunc func(currentThreshold int, oldInputValue, newInputValue InputType) int

	ValueReceptor[int]
}

func NewThresholdTransformer[InputType comparable](updateFunc ...func(currentThreshold int, oldInputValue, newInputValue InputType) int) *ThresholdTransformer[InputType] {
	return &ThresholdTransformer[InputType]{
		ValueReceptor: NewValueReceptor[int](),
		updateFunc: lo.First(updateFunc, func(currentThreshold int, _, newInputValue InputType) int {
			var zeroValue InputType
			if newInputValue != zeroValue {
				return currentThreshold + 1
			}

			return currentThreshold - 1
		}),
	}
}

func (t *ThresholdTransformer[InputType]) Track(input ReadOnlyValueReceptor[InputType]) (unsubscribe func()) {
	return input.OnUpdate(func(oldInputValue, newInputValue InputType) {
		t.ValueReceptor.Compute(func(currentThreshold int) int {
			return t.updateFunc(currentThreshold, oldInputValue, newInputValue)
		})
	})
}
