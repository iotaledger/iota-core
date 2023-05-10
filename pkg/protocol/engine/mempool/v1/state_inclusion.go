package mempoolv1

// StateInclusion is used to track the inclusion state of outputs.
type StateInclusion struct {
	*Inclusion
}

// NewStateInclusion creates a new StateInclusion.
func NewStateInclusion() *StateInclusion {
	return &StateInclusion{
		Inclusion: NewInclusion(),
	}
}

// dependsOnCreatingTransaction sets the callbacks on the given transaction to update this StateInclusion.
func (s *StateInclusion) dependsOnCreatingTransaction(transaction *TransactionMetadata) *StateInclusion {
	if transaction != nil {
		transaction.OnAccepted(s.setAccepted)
		transaction.OnRejected(s.setRejected)
		transaction.OnCommitted(s.setCommitted)
		transaction.OnOrphaned(s.setOrphaned)
	}

	return s
}
