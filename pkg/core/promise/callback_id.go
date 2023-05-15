package promise

// CallbackID is an identifier for a callback.
type UniqueID uint64

func (u *UniqueID) Next() UniqueID {
	*u++

	return *u
}

// uniqueCallbackIDCounter is used to generate unique callback IDs.
var uniqueCallbackIDCounter UniqueID
