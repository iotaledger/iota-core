package agential

import "github.com/iotaledger/iota-core/pkg/core/promise"

func Transformer[TargetType, InputType1 comparable](derivedProperty *promise.Value[TargetType], compute func(y InputType1) TargetType, input1 *promise.Value[InputType1]) {
	input1.OnUpdate(func(_, input1 InputType1) {
		derivedProperty.Compute(func(_ TargetType) TargetType { return compute(input1) })
	})
}
