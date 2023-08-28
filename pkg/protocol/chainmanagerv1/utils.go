package chainmanagerv1

// zeroValueIfNil turns a getter function into a getter function that returns the zero value of the return type if the
// object is nil.
func zeroValueIfNil[ObjectType, ReturnType any](getterFunc func(*ObjectType) ReturnType) func(*ObjectType) ReturnType {
	return func(obj *ObjectType) (returnValue ReturnType) {
		if obj == nil {
			return returnValue
		}

		return getterFunc(obj)
	}
}
