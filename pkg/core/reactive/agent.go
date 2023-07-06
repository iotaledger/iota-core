package reactive

// Agent is the interface that describes the minimal functionality of an agent as an entity that is separated from its
// environment.
type Agent interface {
	// Constructed returns a Value that is used to indicate when the agent has instantiated all of its components.
	Constructed() Value[bool]
}
