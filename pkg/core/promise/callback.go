package promise

import (
	"sync"
)

// Callback is a wrapper for a callback function that is extended by an ID and a mutex.
type Callback[T any] struct {
	// ID is the unique identifier of the callback.
	ID UniqueID

	// Invoke is the callback function that is invoked when the callback is triggered.
	Invoke T

	// unsubscribed is a flag that indicates whether the callback was unsubscribed.
	unsubscribed bool

	// lastUpdate is the last update that was applied to the callback.
	lastUpdate UniqueID

	// mutex is the mutex that is used to ensure that the callback is not triggered concurrently.
	mutex sync.Mutex
}

// NewCallback is the constructor for the Callback type.
func NewCallback[T any](id UniqueID, invoke T) *Callback[T] {
	return &Callback[T]{
		ID:     id,
		Invoke: invoke,
	}
}

// Lock locks the callback for the given update and returns true if the callback was locked successfully.
func (c *Callback[T]) Lock(updateID UniqueID) bool {
	c.mutex.Lock()

	if c.unsubscribed || updateID != 0 && updateID == c.lastUpdate {
		c.mutex.Unlock()

		return false
	}

	c.lastUpdate = updateID

	return true
}

// Unlock unlocks the callback.
func (c *Callback[T]) Unlock() {
	c.mutex.Unlock()
}

// MarkUnsubscribed marks the callback as unsubscribed (it will no longer trigger).
func (c *Callback[T]) MarkUnsubscribed() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.unsubscribed = true
}
