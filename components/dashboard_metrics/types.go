package dashboardmetrics

import "fmt"

// ComponentType defines the component for the different BPS metrics.
type ComponentType byte

const (
	// Received denotes blocks received from the network.
	Received ComponentType = iota
	// Issued denotes blocks that the node itself issued.
	Issued
	// Allowed denotes blocks that passed the filter checks.
	Allowed
	// Attached denotes blocks stored by the block store.
	Attached
	// Solidified denotes blocks solidified by the solidifier.
	Solidified
	// Scheduled denotes blocks scheduled by the scheduler.
	Scheduled
	// SchedulerDropped denotes blocks dropped by the scheduler.
	SchedulerDropped
	// SchedulerSkipped denotes confirmed blocks skipped by the scheduler.
	SchedulerSkipped
	// Booked denotes blocks booked by the booker.
	Booked
)

// NodeInfoExtended represents extended information about the node.
type NodeInfoExtended struct {
	Version       string `json:"version"`
	LatestVersion string `json:"latestVersion"`
	Uptime        int64  `json:"uptime"`
	NodeID        string `json:"nodeId"`
	NodeAlias     string `json:"nodeAlias"`
	MemoryUsage   int64  `json:"memUsage"`
}

// DatabaseSizesMetric represents database size metrics.
type DatabaseSizesMetric struct {
	Prunable  int64 `json:"prunable"`
	Permanent int64 `json:"permanent"`
	Total     int64 `json:"total"`
	Time      int64 `json:"ts"`
}

// String returns the stringified component type.
func (c ComponentType) String() string {
	switch c {
	case Received:
		return "Received"
	case Issued:
		return "Issued"
	case Allowed:
		return "Allowed"
	case Attached:
		return "Attached"
	case Solidified:
		return "Solidified"
	case Scheduled:
		return "Scheduled"
	case SchedulerDropped:
		return "SchedulerDropped"
	case SchedulerSkipped:
		return "SchedulerSkipped"
	case Booked:
		return "Booked"
	default:
		return fmt.Sprintf("Unknown (%d)", c)
	}
}
