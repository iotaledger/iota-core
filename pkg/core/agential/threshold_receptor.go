package agential

import "github.com/iotaledger/hive.go/lo"

type ThresholdReceptor[OutputType, InputType comparable] struct {
	Receptor[int]
	updateFunc func(currentThreshold int, oldInputValue, newInputValue InputType) int
}

func NewThresholdReceptor[OutputType, InputType comparable](updateFunc ...func(currentThreshold int, oldInputValue, newInputValue InputType) int) *ThresholdReceptor[OutputType, InputType] {
	return &ThresholdReceptor[OutputType, InputType]{
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

func (t *ThresholdReceptor[OutputType, InputType]) Get() int {
	return t.Receptor.Get()
}

func (t *ThresholdReceptor[OutputType, InputType]) OnUpdate(consumer func(oldValue, newValue int)) (unsubscribe func()) {
	return t.Receptor.OnUpdate(consumer)
}

func (t *ThresholdReceptor[OutputType, InputType]) ProvideInput(input ReadOnlyReceptor[InputType]) (unsubscribe func()) {
	return input.OnUpdate(func(oldInputValue, newInputValue InputType) {
		t.Receptor.Compute(func(currentThreshold int) int {
			return t.updateFunc(currentThreshold, oldInputValue, newInputValue)
		})
	})
}
