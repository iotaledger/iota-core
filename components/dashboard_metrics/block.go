package dashboardmetrics

import (
	"go.uber.org/atomic"

	"github.com/iotaledger/hive.go/runtime/syncutils"
)

// the same metrics as above, but since the start of a node.
var (
	// Number of blocks per component (store, scheduler, booker) type since start of the node.
	// One for dashboard (reset every time is read), other for grafana with cumulative value.
	blockCountPerComponentDashboard = make(map[ComponentType]uint64)

	// protect map from concurrent read/write.
	blockCountPerComponentMutex syncutils.RWMutex

	// counter for the received BPS (for dashboard).
	mpsAttachedSinceLastMeasurement atomic.Uint64
)

// blockCountSinceStartPerComponentDashboard returns a map of block count per component types and their count since last time the value was read.
func blockCountSinceStartPerComponentDashboard() map[ComponentType]uint64 {
	blockCountPerComponentMutex.RLock()
	defer blockCountPerComponentMutex.RUnlock()

	// copy the original map
	clone := make(map[ComponentType]uint64)
	for key, element := range blockCountPerComponentDashboard {
		clone[key] = element
	}

	return clone
}

// measures the Component Counter value per second.
func measurePerComponentCounter() {
	// sample the current counter value into a measured BPS value
	componentCounters := blockCountSinceStartPerComponentDashboard()

	// reset the counter
	blockCountPerComponentMutex.Lock()
	for key := range blockCountPerComponentDashboard {
		blockCountPerComponentDashboard[key] = 0
	}
	blockCountPerComponentMutex.Unlock()

	// trigger events for outside listeners
	Events.ComponentCounterUpdated.Trigger(&ComponentCounterUpdatedEvent{ComponentStatus: componentCounters})
}

// measures the received BPS value.
func measureAttachedBPS() {
	// sample the current counter value into a measured BPS value
	sampledBPS := mpsAttachedSinceLastMeasurement.Load()

	// reset the counter
	mpsAttachedSinceLastMeasurement.Store(0)

	// trigger events for outside listeners
	Events.AttachedBPSUpdated.Trigger(&AttachedBPSUpdatedEvent{BPS: sampledBPS})
}

// increases the received BPS counter.
func increaseReceivedBPSCounter() {
	mpsAttachedSinceLastMeasurement.Inc()
}
