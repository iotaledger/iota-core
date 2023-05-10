package mempoolv1

// StateInclusion is used to track the inclusion state of outputs.
type StateInclusion struct {
	*Inclusion
}

// NewStateInclusion creates a new StateInclusion.
func NewStateInclusion(creatingTransaction ...*TransactionMetadata) *StateInclusion {
	s := &StateInclusion{
		Inclusion: NewInclusion(),
	}

	if len(creatingTransaction) > 0 {
		creatingTransaction[0].OnPending(s.setPending)
		creatingTransaction[0].OnAccepted(s.setAccepted)
		creatingTransaction[0].OnRejected(s.setRejected)
		creatingTransaction[0].OnCommitted(s.setCommitted)
		creatingTransaction[0].OnOrphaned(s.setOrphaned)
	}

	return s
}
