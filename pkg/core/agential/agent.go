package agential

// Agent is the interface that describes the minimal functionality of an agent as an entity that is separated from its
// environment.
type Agent interface {
	// Constructed returns a ValueReceptor that is used to signal that the agent has been constructed.
	Constructed() Value[bool]
}
