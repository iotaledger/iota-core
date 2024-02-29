package inspection

// MapInspector is a utility function that can be used to inspect a map of elements.
func MapInspector[K comparable, V any](mapToInspect mapInterface[K, V], inspect func(inspectedMap InspectedObject, key K, value V)) func(inspectedMap InspectedObject) {
	return func(inspectedMap InspectedObject) {
		mapToInspect.ForEach(func(key K, value V) bool {
			inspect(inspectedMap, key, value)

			return true
		})
	}
}

// mapInterface is an interface that is used to iterate over a map of elements.
type mapInterface[K comparable, V any] interface {
	ForEach(consumer func(key K, value V) bool)
}
