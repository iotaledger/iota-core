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
