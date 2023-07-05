package agential

import "github.com/iotaledger/hive.go/lo"

// Counter is a ValueReceptor that counts the number of times the tracked input values fulfill a certain condition.
type Counter[InputType comparable] struct {
	// ValueReceptor is the ValueReceptor that holds the output value of the Counter.
	*ValueReceptor[int]

	// condition is the condition that is used to determine whether the input value fulfills the counted criteria.
	condition func(inputValue InputType) bool
}

// NewCounter creates a new Counter that counts the number of times a certain input value fulfills a condition.
func NewCounter[InputType comparable](condition ...func(inputValue InputType) bool) *Counter[InputType] {
	return &Counter[InputType]{
		ValueReceptor: NewValueReceptor[int](),
		condition: lo.First(condition, func(newInputValue InputType) bool {
			var zeroValue InputType
			return newInputValue != zeroValue
		}),
	}
}

// Count adds the given input value receptor as an input to the Counter and returns a function that can be used to
// unsubscribe the input value receptor.
func (t *Counter[InputType]) Count(input Value[InputType]) (unsubscribe func()) {
	return input.OnUpdate(func(oldInputValue, newInputValue InputType) {
		t.ValueReceptor.Compute(func(currentThreshold int) int {
			if t.condition(newInputValue) {
				return currentThreshold + 1
			} else {
				return currentThreshold - 1
			}
		})
	})
}
