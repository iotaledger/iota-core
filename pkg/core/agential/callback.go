package agential

import (
	"sync"

	"github.com/iotaledger/iota-core/pkg/core/types"
)

// callback is a wrapper for a callback function that is extended by an ID and a mutex to ensure that the same callback
// can not be triggered concurrently.
type callback[T any] struct {
	// ID is the unique identifier of the callback.
	ID types.UniqueID

	// Invoke is the callback function that is invoked when the callback is triggered.
	Invoke T

	// unsubscribed is a flag that indicates whether the callback was unsubscribed.
	unsubscribed bool

	// lastUpdate is the last update that was applied to the callback.
	lastUpdate types.UniqueID

	// mutex is the mutex that is used to ensure that the callback is not triggered concurrently.
	mutex sync.Mutex
}

// newCallback is the constructor for the callback type.
func newCallback[T any](id types.UniqueID, invoke T) *callback[T] {
	return &callback[T]{
		ID:     id,
		Invoke: invoke,
	}
}

// Lock locks the callback for the given update and returns true if the callback was locked successfully.
func (c *callback[T]) Lock(updateID types.UniqueID) bool {
	c.mutex.Lock()

	if c.unsubscribed || updateID != 0 && updateID == c.lastUpdate {
		c.mutex.Unlock()

		return false
	}

	c.lastUpdate = updateID

	return true
}

// Unlock unlocks the callback.
func (c *callback[T]) Unlock() {
	c.mutex.Unlock()
}

// MarkUnsubscribed marks the callback as unsubscribed (it will no longer trigger).
func (c *callback[T]) MarkUnsubscribed() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.unsubscribed = true
}
