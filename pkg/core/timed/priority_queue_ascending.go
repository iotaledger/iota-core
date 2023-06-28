package timed

import (
	"time"

	"github.com/iotaledger/iota-core/pkg/core/priorityqueue"
)

// priorityQueueAscending is a priority queue that sorts elements by their time in ascending order.
type priorityQueueAscending[T any] struct {
	*priorityqueue.PriorityQueue[T, timeAscending]
}

// Push adds an element to the queue with the given time.
func (a *priorityQueueAscending[T]) Push(element T, time time.Time) {
	a.PriorityQueue.Push(element, timeAscending(time))
}

// PopUntil removes elements from the top of the queue until the given time.
func (a *priorityQueueAscending[T]) PopUntil(time time.Time) []T {
	return a.PriorityQueue.PopUntil(timeAscending(time))
}

// timeAscending is a wrapper around time.Time that implements the Comparable interface for ascending order.
type timeAscending time.Time

// CompareTo compares the time of the receiver to the time of the given other timeAscending.
func (t timeAscending) CompareTo(other timeAscending) int {
	switch {
	case time.Time(t).Before(time.Time(other)):
		return -1
	case time.Time(t).After(time.Time(other)):
		return 1
	default:
		return 0
	}
}
