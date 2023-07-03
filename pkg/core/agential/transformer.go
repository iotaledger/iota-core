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

func NewTransformerWith2Inputs[TargetType, InputType1, InputType2 comparable](transitionFunc func(InputType1, InputType2) TargetType) *TransformerWith2Inputs[TargetType, InputType1, InputType2] {
	return &TransformerWith2Inputs[TargetType, InputType1, InputType2]{
		receptor:       newReceptor[TargetType](),
		transitionFunc: transitionFunc,
	}
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
