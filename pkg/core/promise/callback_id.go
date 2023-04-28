package promise

import "sync/atomic"

// CallbackID is an identifier for a callback.
type CallbackID = uint64

// NewCallbackID creates a new unique callback ID.
func NewCallbackID() CallbackID {
	return atomic.AddUint64(&uniqueCallbackIDCounter, 1)
}

// uniqueCallbackIDCounter is used to generate unique callback IDs.
var uniqueCallbackIDCounter CallbackID
