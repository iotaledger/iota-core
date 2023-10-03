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

func (s *SignedTransaction) String() string {
	return "SignedTransaction(" + s.id.String() + ")"
}

type Transaction struct {
	id                 iotago.TransactionID
	utxoInputs         []iotago.Input
	contextInputs      []iotago.Input
	outputCount        uint16
	invalidTransaction bool
}

func NewSignedTransaction(transaction mempool.Transaction) *SignedTransaction {
	return &SignedTransaction{
		id:          tpkg.RandSignedTransactionID(),
		transaction: transaction,
	}
}

func NewTransaction(outputCount uint16, inputs []iotago.Input, contextInputs ...iotago.Input) *Transaction {
	return &Transaction{
		id:            tpkg.RandTransactionID(),
		utxoInputs:    inputs,
		contextInputs: contextInputs,
		outputCount:   outputCount,
	}
}

func (t *Transaction) ID() (iotago.TransactionID, error) {
	return t.id, nil
}

func (t *Transaction) Inputs() ([]iotago.Input, error) {
	return append(t.utxoInputs, t.contextInputs...), nil
}

func (t *Transaction) UTXOInputs() ([]iotago.Input, error) {
	return t.utxoInputs, nil
}

func (t *Transaction) ContextInputs() ([]iotago.Input, error) {
	return t.contextInputs, nil
}

func (t *Transaction) String() string {
	return "Transaction(" + t.id.String() + ")"
}

var _ mempool.Transaction = new(Transaction)
