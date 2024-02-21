package daemon

// Please add the dependencies if you add your own priority here.
// Otherwise investigating deadlocks at shutdown is much more complicated.

const (
	PriorityCloseDatabase = iota // no dependencies
	PriorityP2P
	PriorityProtocol
	PriorityRestAPI
	PriorityINX
	PriorityDashboardMetrics
	PriorityDashboard
	PriorityMetrics
)
