package daemon

// Please add the dependencies if you add your own priority here.
// Otherwise investigating deadlocks at shutdown is much more complicated.

const (
	PriorityCloseDatabase = iota // no dependencies
	PriorityPeerDatabase
	PriorityP2P
	PriorityManualPeering
	PriorityRestAPI // depends on PriorityPoWHandler
)
