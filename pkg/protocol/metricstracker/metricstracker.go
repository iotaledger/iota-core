package metricstracker

import "github.com/iotaledger/hive.go/runtime/module"

type MetricsTracker interface {
	// SyncStatus returns the sync status of a node.
	NodeMetrics() *NodeMetrics

	// Shutdown shuts down the MetricsTracker.
	Shutdown()

	module.Interface
}

type NodeMetrics struct {
	BlocksPerSecond          float64
	ConfirmedBlocksPerSecond float64
	ConfirmedRate            float64
}
