package mempooltests

import (
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

type Transaction struct {
	id                 iotago.TransactionID
	inputs             []iotago.IndexedUTXOReferencer
	outputCount        uint16
	invalidTransaction bool
}

func NewTransaction(outputCount uint16, inputs ...iotago.IndexedUTXOReferencer) *Transaction {
	return &Transaction{
		id:          tpkg.RandTransactionID(),
		inputs:      inputs,
		outputCount: outputCount,
	}
}

func (t *Transaction) ID() (iotago.TransactionID, error) {
	return t.id, nil
}

func (t *Transaction) Inputs() ([]iotago.IndexedUTXOReferencer, error) {
	return t.inputs, nil
}

func (t *Transaction) String() string {
	return "Transaction(" + t.id.String() + ")"
}

var _ mempool.Transaction = new(Transaction)
