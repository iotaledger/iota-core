package mempool

type StateLifecycle interface {
	IsSpent() bool

	OnDoubleSpent(callback func())

	OnSpendAccepted(callback func(spender TransactionMetadata))

	OnSpendCommitted(callback func(spender TransactionMetadata))

	AllSpendersRemoved() bool

	OnAllSpendersRemoved(callback func()) (unsubscribe func())

	SpenderCount() uint64

	HasNoSpenders() bool
}
