package agential

import "github.com/iotaledger/hive.go/lo"

type TransformerWith1Input[TargetType, InputType1 comparable] struct {
	*receptor[TargetType]

	transitionFunc func(InputType1) TargetType
}

func NewTransformerWith1Input[TargetType, InputType1 comparable, InputSource1 ReadOnlyReceptor[InputType1]](agent Agent, transitionFunc func(InputType1) TargetType, input1 *InputSource1) *TransformerWith1Input[TargetType, InputType1] {
	t := &TransformerWith1Input[TargetType, InputType1]{
		receptor:       newReceptor[TargetType](),
		transitionFunc: transitionFunc,
	}

	agent.OnConstructed(func() {
		(*input1).OnUpdate(func(_, input1 InputType1) {
			t.Compute(func(_ TargetType) TargetType { return t.transitionFunc(input1) })
		})
	})

	return t
}

type TransformerWith2Inputs[TargetType, InputType1, InputType2 comparable] struct {
	*receptor[TargetType]

	transitionFunc func(InputType1, InputType2) TargetType
}

func NewTransformerWith2Inputs[TargetType, InputType1, InputType2 comparable, InputSource1 ReadOnlyReceptor[InputType1], InputSource2 ReadOnlyReceptor[InputType2]](agent Agent, transitionFunc func(InputType1, InputType2) TargetType, input1 *InputSource1, input2 *InputSource2) *TransformerWith2Inputs[TargetType, InputType1, InputType2] {
	t := &TransformerWith2Inputs[TargetType, InputType1, InputType2]{
		receptor:       newReceptor[TargetType](),
		transitionFunc: transitionFunc,
	}

	agent.OnConstructed(func() {
		(*input1).OnUpdate(func(_, input1 InputType1) {
			t.Compute(func(_ TargetType) TargetType { return t.transitionFunc(input1, (*input2).Get()) })
		})

		(*input2).OnUpdate(func(_, input2 InputType2) {
			t.Compute(func(_ TargetType) TargetType { return t.transitionFunc((*input1).Get(), input2) })
		})
	})

	return t
}

type TransformerWith3Inputs[TargetType, InputType1, InputType2, InputType3 comparable] struct {
	*receptor[TargetType]

	transitionFunc func(InputType1, InputType2, InputType3) TargetType
}

func NewTransformerWith3Inputs[TargetType, InputType1, InputType2, InputType3 comparable, InputSource1 ReadOnlyReceptor[InputType1], InputSource2 ReadOnlyReceptor[InputType2], InputSource3 ReadOnlyReceptor[InputType3]](agent Agent, transitionFunc func(InputType1, InputType2, InputType3) TargetType, input1 *InputSource1, input2 *InputSource2, input3 *InputSource3) *TransformerWith3Inputs[TargetType, InputType1, InputType2, InputType3] {
	t := &TransformerWith3Inputs[TargetType, InputType1, InputType2, InputType3]{
		receptor:       newReceptor[TargetType](),
		transitionFunc: transitionFunc,
	}

	agent.OnConstructed(func() {
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

	Receptor[int]
}

func NewThresholdTransformer[InputType comparable](updateFunc ...func(currentThreshold int, oldInputValue, newInputValue InputType) int) *ThresholdTransformer[InputType] {
	return &ThresholdTransformer[InputType]{
		Receptor: NewReceptor[int](),
		updateFunc: lo.First(updateFunc, func(currentThreshold int, _, newInputValue InputType) int {
			var zeroValue InputType

			if newInputValue != zeroValue {
				return currentThreshold + 1
			} else {
				return currentThreshold - 1
			}
		}),
	}
}

func (t *ThresholdTransformer[InputType]) Track(input ReadOnlyReceptor[InputType]) (unsubscribe func()) {
	return input.OnUpdate(func(oldInputValue, newInputValue InputType) {
		t.Receptor.Compute(func(currentThreshold int) int {
			return t.updateFunc(currentThreshold, oldInputValue, newInputValue)
		})
	})
}
