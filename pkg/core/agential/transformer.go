package agential

import "github.com/iotaledger/hive.go/lo"

type TransformerWith1Input[TargetType, InputType1 comparable] struct {
	*receptor[TargetType]

	transitionFunc func(InputType1) TargetType
}

func NewTransformerWith1Input[TargetType, InputType1 comparable](transitionFunc func(InputType1) TargetType) *TransformerWith1Input[TargetType, InputType1] {
	return &TransformerWith1Input[TargetType, InputType1]{
		receptor:       newReceptor[TargetType](),
		transitionFunc: transitionFunc,
	}
}

func (d *TransformerWith1Input[TargetType, InputType1]) ProvideInputs(input1 ReadOnlyReceptor[InputType1]) (unsubscribe func()) {
	return lo.Batch(
		input1.OnUpdate(func(_, input1 InputType1) {
			d.Compute(func(_ TargetType) TargetType { return d.transitionFunc(input1) })
		}),
	)
}

type TransformerWith2Inputs[TargetType, InputType1, InputType2 comparable] struct {
	*receptor[TargetType]

	transitionFunc func(InputType1, InputType2) TargetType
}

func NewTransformerWith2Inputs[TargetType, InputType1, InputType2 comparable, InputSource1 ReadOnlyReceptor[InputType1], InputSource2 ReadOnlyReceptor[InputType2]](agent Agent, transitionFunc func(InputType1, InputType2) TargetType, input1 func() InputSource1, input2 func() InputSource2) *TransformerWith2Inputs[TargetType, InputType1, InputType2] {
	t := &TransformerWith2Inputs[TargetType, InputType1, InputType2]{
		receptor:       newReceptor[TargetType](),
		transitionFunc: transitionFunc,
	}

	agent.OnConstructed(func() { t.ProvideInputs(input1(), input2()) })

	return t
}

func (d *TransformerWith2Inputs[TargetType, InputType1, InputType2]) ProvideInputs(input1 ReadOnlyReceptor[InputType1], input2 ReadOnlyReceptor[InputType2]) (unsubscribe func()) {
	return lo.Batch(
		input1.OnUpdate(func(_, input1 InputType1) {
			d.Compute(func(_ TargetType) TargetType { return d.transitionFunc(input1, input2.Get()) })
		}),

		input2.OnUpdate(func(_, input2 InputType2) {
			d.Compute(func(_ TargetType) TargetType { return d.transitionFunc(input1.Get(), input2) })
		}),
	)
}

type TransformerWith3Inputs[TargetType, InputType1, InputType2, InputType3 comparable] struct {
	*receptor[TargetType]

	transitionFunc func(InputType1, InputType2, InputType3) TargetType
}

func NewTransformerWith3Inputs[TargetType, InputType1, InputType2, InputType3 comparable](transitionFunc func(InputType1, InputType2, InputType3) TargetType) *TransformerWith3Inputs[TargetType, InputType1, InputType2, InputType3] {
	return &TransformerWith3Inputs[TargetType, InputType1, InputType2, InputType3]{
		receptor:       newReceptor[TargetType](),
		transitionFunc: transitionFunc,
	}
}

func (d *TransformerWith3Inputs[TargetType, InputType1, InputType2, InputType3]) ProvideInputs(input1 ReadOnlyReceptor[InputType1], input2 ReadOnlyReceptor[InputType2], input3 ReadOnlyReceptor[InputType3]) (unsubscribe func()) {
	return lo.Batch(
		input1.OnUpdate(func(_, input1 InputType1) {
			d.Compute(func(_ TargetType) TargetType { return d.transitionFunc(input1, input2.Get(), input3.Get()) })
		}),

		input2.OnUpdate(func(_, input2 InputType2) {
			d.Compute(func(_ TargetType) TargetType { return d.transitionFunc(input1.Get(), input2, input3.Get()) })
		}),

		input3.OnUpdate(func(_, input3 InputType3) {
			d.Compute(func(_ TargetType) TargetType { return d.transitionFunc(input1.Get(), input2.Get(), input3) })
		}),
	)
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

func (t *ThresholdTransformer[InputType]) ProvideInput(input ReadOnlyReceptor[InputType]) (unsubscribe func()) {
	return input.OnUpdate(func(oldInputValue, newInputValue InputType) {
		t.Receptor.Compute(func(currentThreshold int) int {
			return t.updateFunc(currentThreshold, oldInputValue, newInputValue)
		})
	})
}
