package mockedvm

import (
	iotago2 "iota-core/pkg/iotago"

	"github.com/iotaledger/hive.go/stringify"
	iotago "github.com/iotaledger/iota.go/v4"
)

// MockedInput is a mocked entity that allows to "address" which Outputs are supposed to be used by a Transaction.
type MockedInput struct {
	// outputID contains the referenced OutputID.
	OutputID iotago.OutputID `serix:"0"`
}

// NewMockedInput creates a new MockedInput from an utxo.OutputID.
func NewMockedInput(outputID iotago.OutputID) *MockedInput {
	return &MockedInput{OutputID: outputID}
}

func (m *MockedInput) ID() iotago.OutputID {
	return m.OutputID
}

// String returns a human-readable version of the MockedInput.
func (m *MockedInput) String() (humanReadable string) {
	return stringify.Struct("MockedInput",
		stringify.NewStructField("OutputID", m.OutputID),
	)
}

// utxoInput type-casts the MockedInput to a utxo.Input.
func (m *MockedInput) utxoInput() (input iotago2.Input) {
	return m
}

// code contract (make sure the struct implements all required methods).
var _ iotago2.Input = new(MockedInput)
