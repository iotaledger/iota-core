package agential

import "github.com/iotaledger/hive.go/lo"

type ThresholdReceptor[InputType comparable] struct {
	Receptor[int]
	updateFunc func(currentThreshold int, oldInputValue, newInputValue InputType) int
}

func NewThresholdReceptor[InputType comparable](updateFunc ...func(currentThreshold int, oldInputValue, newInputValue InputType) int) *ThresholdReceptor[InputType] {
	return &ThresholdReceptor[InputType]{
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

func (t *ThresholdReceptor[InputType]) Get() int {
	return t.Receptor.Get()
}

func (t *ThresholdReceptor[InputType]) OnUpdate(consumer func(oldValue, newValue int)) (unsubscribe func()) {
	return t.Receptor.OnUpdate(consumer)
}

func (t *ThresholdReceptor[InputType]) ProvideInput(input ReadOnlyReceptor[InputType]) (unsubscribe func()) {
	return input.OnUpdate(func(oldInputValue, newInputValue InputType) {
		t.Receptor.Compute(func(currentThreshold int) int {
			return t.updateFunc(currentThreshold, oldInputValue, newInputValue)
		})
	})
}
