package reactive

import "github.com/iotaledger/hive.go/lo"

// Counter is a Variable that derives its value from the number of times the monitored input values fulfill a certain
// condition.
type Counter[InputType comparable] interface {
	// Variable holds the counter value.
	Variable[int]

	// Monitor adds the given input value as an input to the counter and returns a function that can be used to
	// unsubscribe from the input value.
	Monitor(input ReadableVariable[InputType]) (unsubscribe func())
}

// NewCounter creates a Counter that counts the number of times monitored input values fulfill a certain condition.
func NewCounter[InputType comparable](condition ...func(inputValue InputType) bool) Counter[InputType] {
	return &counter[InputType]{
		Variable: NewVariable[int](),
		condition: lo.First(condition, func(newInputValue InputType) bool {
			var zeroValue InputType
			return newInputValue != zeroValue
		}),
	}
}

// counter is the default implementation of the Counter interface.
type counter[InputType comparable] struct {
	// Variable holds the counter value.
	Variable[int]

	// condition is the condition that is used to determine whether the input value fulfills the counted criteria.
	condition func(inputValue InputType) bool
}

// Monitor adds the given input value as an input to the counter and returns a function that can be used to unsubscribe
// from the input value.
func (c *counter[InputType]) Monitor(input ReadableVariable[InputType]) (unsubscribe func()) {
	var conditionWasTrue bool

	return input.OnUpdate(func(_, newInputValue InputType) {
		c.Compute(func(currentValue int) int {
			if conditionIsTrue := c.condition(newInputValue); conditionIsTrue != conditionWasTrue {
				if conditionIsTrue {
					currentValue++
				} else {
					currentValue--
				}

				conditionWasTrue = conditionIsTrue
			}

			return currentValue
		})
	}, true)
}
