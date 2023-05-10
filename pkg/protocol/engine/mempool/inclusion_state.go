package mempool

type InclusionState interface {
	IsAccepted() bool

	OnAccepted(callback func())

	IsCommitted() bool

	OnCommitted(callback func())

	IsRejected() bool

	OnRejected(callback func())

	IsOrphaned() bool

	OnOrphaned(callback func())
}
