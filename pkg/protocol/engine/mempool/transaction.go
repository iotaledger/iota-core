package mempool

// Transaction is the type that is used to describe instructions how to modify the ledger state.
type Transaction interface {
	// ID returns the identifier of the Transaction.
	ID() (TransactionID, error)

	// Inputs returns the inputs of the Transaction.
	Inputs() ([]Input, error)

	// String returns a human-readable version of the Transaction.
	String() string
}
