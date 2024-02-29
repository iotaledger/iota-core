package inspection

type IterableSet[T comparable] interface {
	ForEach(consumer func(element T) error) error
}

func InspectSet[T comparable](setToInspect IterableSet[T], inspect func(inspectedSet InspectedObject, element T)) func(inspectedSet InspectedObject) {
	return func(inspectedSet InspectedObject) {
		_ = setToInspect.ForEach(func(key T) error {
			inspect(inspectedSet, key)

			return nil
		})
	}
}

type IterableMap[K comparable, V any] interface {
	ForEach(consumer func(key K, value V) bool)
}

func InspectMap[K comparable, V any](mapToInspect IterableMap[K, V], inspect func(inspectedMap InspectedObject, key K, value V)) func(inspectedMap InspectedObject) {
	return func(inspectedMap InspectedObject) {
		mapToInspect.ForEach(func(key K, value V) bool {
			inspect(inspectedMap, key, value)

			return true
		})
	}
}
