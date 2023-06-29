package agential

import "github.com/iotaledger/iota-core/pkg/core/promise"

func Gate2[TargetType, InputType1, InputType2 comparable](derivedProperty *promise.Value[TargetType], compute func(y InputType1, z InputType2) TargetType, input1 *promise.Value[InputType1], input2 *promise.Value[InputType2]) {
	input1.OnUpdate(func(_, input1 InputType1) {
		derivedProperty.Compute(func(_ TargetType) TargetType { return compute(input1, input2.Get()) })
	})

	input2.OnUpdate(func(_, input2 InputType2) {
		derivedProperty.Compute(func(_ TargetType) TargetType { return compute(input1.Get(), input2) })
	})
}
