package mempooltests

import (
	"iota-core/pkg/protocol/engine/mempool"
	"iota-core/pkg/protocol/engine/vm"

	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

type Transaction struct {
	id          iotago.TransactionID
	inputs      []mempool.StateReference
	outputCount uint16
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

func (t *Transaction) Inputs() ([]mempool.StateReference, error) {
	return t.inputs, nil
}

func (t *Transaction) String() string {
	return "Transaction(" + t.id.String() + ")"
}

var _ vm.StateTransition = new(Transaction)
