package promise

type Int struct {
	*Value[int]
}

func NewInt() *Int {
	return &Int{NewValue[int]()}
}

func (i *Int) Increase() int {
	return i.Compute(func(currentValue int) int {
		return currentValue + 1
	})
}

func (i *Int) Decrease() int {
	return i.Compute(func(currentValue int) int {
		return currentValue - 1
	})
}
