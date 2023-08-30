package chainmanagerv1

// noPanicIfNil prevents a panic of the getter when the receiver is nil by returning the zero value of the return type.
func noPanicIfNil[ReceiverType, ReturnType any](getter func(*ReceiverType) ReturnType) func(*ReceiverType) ReturnType {
	return func(receiver *ReceiverType) (zeroValue ReturnType) {
		if receiver == nil {
			return zeroValue
		}

		return getter(receiver)
	}
}
