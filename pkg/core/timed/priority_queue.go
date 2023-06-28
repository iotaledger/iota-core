package timed

import (
	"time"

	"github.com/iotaledger/hive.go/lo"
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

// NewPriorityQueue creates a new PriorityQueue that can optionally be set to ascending order (oldest element first).
func NewPriorityQueue[T any](ascending ...bool) PriorityQueue[T] {
	if lo.First(ascending) {
		return &priorityQueueAscending[T]{
			PriorityQueue: priorityqueue.New[T, timeAscending](),
		}
	}

	return &priorityQueueDescending[T]{
		PriorityQueue: priorityqueue.New[T, timeDescending](),
	}
}
