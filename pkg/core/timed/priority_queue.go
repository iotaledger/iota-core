package timed

import (
	"time"

	"github.com/iotaledger/iota-core/pkg/core/priorityqueue"
)

// PriorityQueue is a priority queue whose elements are sorted by time.
type PriorityQueue[ElementType any] interface {
	// Push adds an element to the queue with the given time.
	Push(element ElementType, time time.Time)

	// Pop removes the element with the highest priority from the queue.
	Pop() (element ElementType, exists bool)

	// PopUntil removes elements from the top of the queue until the given time.
	PopUntil(time time.Time) []ElementType

	// PopAll removes all elements from the queue.
	PopAll() []ElementType

	// Size returns the number of elements in the queue.
	Size() int
}

// NewDescendingPriorityQueue creates a new descending PriorityQueue.
func NewDescendingPriorityQueue[T any]() PriorityQueue[T] {
	return &priorityQueueDescending[T]{
		PriorityQueue: priorityqueue.New[T, timeDescending](),
	}
}

// NewAscendingPriorityQueue creates a new ascending PriorityQueue.
func NewAscendingPriorityQueue[T any]() PriorityQueue[T] {
	return &priorityQueueAscending[T]{
		PriorityQueue: priorityqueue.New[T, timeAscending](),
	}
}
