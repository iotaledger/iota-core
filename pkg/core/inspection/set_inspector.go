package inspection

// SetInspector is a utility function that can be used to inspect a set of elements.
func SetInspector[T comparable](setToInspect setInterface[T], inspect func(inspectedSet InspectedObject, element T)) func(inspectedSet InspectedObject) {
	return func(inspectedSet InspectedObject) {
		_ = setToInspect.ForEach(func(key T) error {
			inspect(inspectedSet, key)

			return nil
		})
	}
}

// setInterface is an interface that is used to iterate over a setInterface of elements.
type setInterface[T comparable] interface {
	ForEach(consumer func(element T) error) error
}
