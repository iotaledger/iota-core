package agential

import "github.com/iotaledger/hive.go/lo"

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

func (t *ThresholdTransformer[InputType]) Track(input ValueReceptorReadOnly[InputType]) (unsubscribe func()) {
	return input.OnUpdate(func(oldInputValue, newInputValue InputType) {
		t.ValueReceptor.Compute(func(currentThreshold int) int {
			return t.updateFunc(currentThreshold, oldInputValue, newInputValue)
		})
	})
}
