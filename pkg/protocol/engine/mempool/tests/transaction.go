package mempooltests

import (
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

type SignedTransaction struct {
	id          iotago.SignedTransactionID
	transaction mempool.Transaction
}

func (s *SignedTransaction) ID() (iotago.SignedTransactionID, error) {
	return s.id, nil
}

func (s *SignedTransaction) MustID() iotago.SignedTransactionID {
	return s.id
}

func (s *SignedTransaction) String() string {
	return "SignedTransaction(" + s.id.String() + ")"
}

type Transaction struct {
	id                 iotago.TransactionID
	inputs             []mempool.StateReference
	outputCount        uint16
	invalidTransaction bool
}

func NewSignedTransaction(transaction mempool.Transaction) *SignedTransaction {
	return &SignedTransaction{
		id:          tpkg.RandSignedTransactionID(),
		transaction: transaction,
	}
}

func NewTransaction(outputCount uint16, inputs ...mempool.StateReference) *Transaction {
	return &Transaction{
		id:          tpkg.RandTransactionID(),
		inputs:      inputs,
		outputCount: outputCount,
	}
}

func (t *Transaction) ID() (iotago.TransactionID, error) {
	return t.id, nil
}

func (t *Transaction) MustID() iotago.TransactionID {
	return t.id
}

func (t *Transaction) Inputs() ([]mempool.StateReference, error) {
	return t.inputs, nil
}

func (t *Transaction) String() string {
	return "Transaction(" + t.id.String() + ")"
}

var _ mempool.Transaction = new(Transaction)
