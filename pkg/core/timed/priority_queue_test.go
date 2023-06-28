package timed

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestAscendingPriorityQueue tests the ascending PriorityQueue.
func TestPriorityQueueAscending(t *testing.T) {
	queue := NewPriorityQueue[int](true)

	now := time.Now()
	queue.Push(1337, now.Add(5*time.Second))
	queue.Push(2774, now.Add(10*time.Second))
	queue.Push(2775, now.Add(11*time.Second))

	requirePopUntil(t, queue, now.Add(5*time.Second), []int{
		1337,
	})

	requirePopUntil(t, queue, now.Add(11*time.Second), []int{
		2774,
		2775,
	})

	requirePopUntil(t, queue, now.Add(12*time.Second), []int{})
}

// TestTimeAscending_CompareTo tests the timeAscending.CompareTo method.
func TestTimeAscending_CompareTo(t *testing.T) {
	now := time.Now()

	require.Equal(t, 0, timeAscending(now).CompareTo(timeAscending(now)))
	require.Equal(t, -1, timeAscending(now.Add(-1*time.Second)).CompareTo(timeAscending(now)))
	require.Equal(t, 1, timeAscending(now.Add(1*time.Second)).CompareTo(timeAscending(now)))
}

// TestDescendingPriorityQueue tests the descending PriorityQueue.
func TestPriorityQueueDescending(t *testing.T) {
	queue := NewPriorityQueue[int]()

	now := time.Now()
	queue.Push(1337, now.Add(5*time.Second))
	queue.Push(2774, now.Add(10*time.Second))
	queue.Push(2775, now.Add(11*time.Second))

	requirePopUntil(t, queue, now.Add(11*time.Second), []int{
		2775,
	})

	requirePopUntil(t, queue, now.Add(5*time.Second), []int{
		2774,
		1337,
	})

	requirePopUntil(t, queue, now.Add(4*time.Second), []int{})
}

// TestTimeDescending_CompareTo tests the timeDescending.CompareTo method.
func TestTimeDescending_CompareTo(t *testing.T) {
	now := time.Now()

	require.Equal(t, 0, timeDescending(now).CompareTo(timeDescending(now)))
	require.Equal(t, 1, timeDescending(now.Add(-1*time.Second)).CompareTo(timeDescending(now)))
	require.Equal(t, -1, timeDescending(now.Add(1*time.Second)).CompareTo(timeDescending(now)))
}

// requirePopUntil asserts that the given queue returns the expected elements when PopUntil is called.
func requirePopUntil[T any](t *testing.T, timedQueue PriorityQueue[T], currentTime time.Time, expectedElements []T) {
	elements := timedQueue.PopUntil(currentTime)
	require.Equal(t, len(expectedElements), len(elements))

	for _, element := range expectedElements {
		require.Contains(t, elements, element)
	}
}
