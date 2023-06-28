package timed

import (
	"time"

	"github.com/iotaledger/iota-core/pkg/core/priorityqueue"
)

// priorityQueueDescending is a priority queue that sorts elements by their time in descending order.
type priorityQueueDescending[T any] struct {
	*priorityqueue.PriorityQueue[T, timeDescending]
}

// Push adds an element to the queue with the given time.
func (d *priorityQueueDescending[T]) Push(element T, time time.Time) {
	d.PriorityQueue.Push(element, timeDescending(time))
}

// PopUntil removes elements from the top of the queue until the given time.
func (d *priorityQueueDescending[T]) PopUntil(time time.Time) []T {
	return d.PriorityQueue.PopUntil(timeDescending(time))
}

// timeDescending is a wrapper around time.Time that implements the Comparable interface for descending order.
type timeDescending time.Time

// CompareTo compares the time of the receiver to the time of the given other timeDescending.
func (t timeDescending) CompareTo(other timeDescending) int {
	switch {
	case time.Time(t).Before(time.Time(other)):
		return 1
	case time.Time(t).After(time.Time(other)):
		return -1
	default:
		return 0
	}
}
