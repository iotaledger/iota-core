package reactive

// Counter is a Variable that derives its value from the number of times a set of monitored input values fulfill a
// certain condition.
type Counter[InputType comparable] interface {
	// Variable holds the counter value.
	Variable[int]

	// Monitor adds the given input value as an input to the counter and returns a function that can be used to
	// unsubscribe from the input value.
	Monitor(input ReadableVariable[InputType]) (unsubscribe func())
}

// NewCounter creates a Counter that counts the number of times monitored input values fulfill a certain condition.
func NewCounter[InputType comparable](condition ...func(inputValue InputType) bool) Counter[InputType] {
	return newCounter(condition...)
}
