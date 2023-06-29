package value

import (
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/core/promise"
)

// DeriveFrom1 makes the target property depend on the input property. Whenever the input property is updated, the
// target property is re-determined by the compute function.
func DeriveFrom1[TargetType, InputType1 comparable](targetProperty *promise.Value[TargetType], compute func(InputType1) TargetType, input1 *promise.Value[InputType1]) (destroy func()) {
	return input1.OnUpdate(func(_, input1 InputType1) {
		targetProperty.Compute(func(_ TargetType) TargetType { return compute(input1) })
	})
}

// DeriveFrom2 makes the target property depend on the 2 input properties. Whenever one of the 2 input
// properties is updated, the target property is re-determined by the compute function.
func DeriveFrom2[TargetType, InputType1, InputType2 comparable](targetProperty *promise.Value[TargetType], compute func(InputType1, InputType2) TargetType, input1 *promise.Value[InputType1], input2 *promise.Value[InputType2]) (destroy func()) {
	return lo.Batch(
		input1.OnUpdate(func(_, input1 InputType1) {
			targetProperty.Compute(func(_ TargetType) TargetType { return compute(input1, input2.Get()) })
		}),

		input2.OnUpdate(func(_, input2 InputType2) {
			targetProperty.Compute(func(_ TargetType) TargetType { return compute(input1.Get(), input2) })
		}),
	)
}

// DeriveFrom3 makes the derived property depend on the 3 input properties. Whenever one of the 3 input
// properties is updated, the derived property is re-determined by the compute function.
func DeriveFrom3[TargetType, InputType1, InputType2, InputType3 comparable](targetProperty *promise.Value[TargetType], compute func(InputType1, InputType2, InputType3) TargetType, input1 *promise.Value[InputType1], input2 *promise.Value[InputType2], input3 *promise.Value[InputType3]) (destroy func()) {
	return lo.Batch(
		input1.OnUpdate(func(_, input1 InputType1) {
			targetProperty.Compute(func(_ TargetType) TargetType { return compute(input1, input2.Get(), input3.Get()) })
		}),

		input2.OnUpdate(func(_, input2 InputType2) {
			targetProperty.Compute(func(_ TargetType) TargetType { return compute(input1.Get(), input2, input3.Get()) })
		}),

		input3.OnUpdate(func(_, input3 InputType3) {
			targetProperty.Compute(func(_ TargetType) TargetType { return compute(input1.Get(), input2.Get(), input3) })
		}),
	)
}
