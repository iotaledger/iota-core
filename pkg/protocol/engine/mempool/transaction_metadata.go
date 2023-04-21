package mempool

type TransactionMetadata interface {
	ID() TransactionID
}
