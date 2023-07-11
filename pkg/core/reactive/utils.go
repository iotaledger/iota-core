package reactive

import (
	"sync"

	"github.com/iotaledger/iota-core/pkg/core/types"
)

// callback is an internal wrapper for a callback function that is extended by an ID and a mutex (for call order
// synchronization).
type callback[FuncType any] struct {
	// ID is the unique identifier of the callback.
	ID types.UniqueID

	// Invoke is the callback function that is invoked when the callback is triggered.
	Invoke FuncType

	// unsubscribed is a flag that indicates whether the callback was unsubscribed.
	unsubscribed bool

	// lastUpdate is the last update that was applied to the callback.
	lastUpdate types.UniqueID

	// executionMutex is the mutex that is used to synchronize the execution of the callback.
	executionMutex sync.Mutex
}

// newCallback is the constructor for the callback type.
func newCallback[FuncType any](id types.UniqueID, invoke FuncType) *callback[FuncType] {
	return &callback[FuncType]{
		ID:     id,
		Invoke: invoke,
	}
}

// LockExecution locks the callback for the given update and returns true if the callback was locked successfully.
func (c *callback[FuncType]) LockExecution(updateID types.UniqueID) bool {
	c.executionMutex.Lock()

	if c.unsubscribed || updateID != 0 && updateID == c.lastUpdate {
		c.executionMutex.Unlock()

		return false
	}

	c.lastUpdate = updateID

	return true
}

// UnlockExecution unlocks the callback.
func (c *callback[FuncType]) UnlockExecution() {
	c.executionMutex.Unlock()
}

// MarkUnsubscribed marks the callback as unsubscribed (it will no longer trigger).
func (c *callback[FuncType]) MarkUnsubscribed() {
	c.executionMutex.Lock()
	defer c.executionMutex.Unlock()

	c.unsubscribed = true
}
