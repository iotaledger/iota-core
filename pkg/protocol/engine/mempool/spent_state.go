package mempool

type SpentState interface {
	IsSpent() bool

	OnDoubleSpent(callback func())

	OnSpendAccepted(callback func(spender TransactionWithMetadata))

	OnSpendCommitted(callback func(spender TransactionWithMetadata))

	AllSpendersRemoved() bool

	OnAllSpendersRemoved(callback func()) (unsubscribe func())

	SpenderCount() uint64

	HasNoSpenders() bool
}
