package mempool

type TransactionInclusion interface {
	AllInputsAccepted() bool

	OnAllInputsAccepted(callback func())

	Commit()

	InclusionState
}
