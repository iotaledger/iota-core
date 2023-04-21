package mockedvm

import (
	iotago2 "iota-core/pkg/iotago"

	iotago "github.com/iotaledger/iota.go/v4"
)

// MockedOutput is the container for the data produced by executing a MockedTransaction.
type MockedOutput struct {
	id iotago.OutputID

	// TxID contains the identifier of the Transaction that created this MockedOutput.
	txID iotago.TransactionID

	// Index contains the Index of the Output in respect to it's creating Transaction (the nth Output will have the
	// Index n).
	index uint16
}

// NewMockedOutput creates a new MockedOutput based on the utxo.TransactionID and its index within the MockedTransaction.
func NewMockedOutput(txID iotago.TransactionID, index uint16) (out *MockedOutput) {
	out = &MockedOutput{
		id:    iotago.OutputIDFromTransactionIDAndIndex(txID, index),
		txID:  txID,
		index: index,
	}
	return out
}

func (m *MockedOutput) ID() iotago.OutputID {
	return m.id
}

// code contract (make sure the struct implements all required methods).
var _ iotago2.Output = new(MockedOutput)
