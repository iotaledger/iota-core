package ledgertests

import (
	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
)

type MockedState struct {
	id           iotago.OutputID
	output       *MockedOutput
	creationTime iotago.SlotIndex
}

func NewMockedState(transactionID iotago.TransactionID, index uint16) *MockedState {
	return &MockedState{
		id:           iotago.OutputIDFromTransactionIDAndIndex(transactionID, index),
		output:       &MockedOutput{},
		creationTime: iotago.SlotIndex(0),
	}
}

func (m *MockedState) StateID() iotago.Identifier {
	return iotago.IdentifierFromData(lo.PanicOnErr(m.id.Bytes()))
}

func (m *MockedState) Type() iotago.StateType {
	return iotago.InputUTXO
}

func (m *MockedState) OutputID() iotago.OutputID {
	return m.id
}

func (m *MockedState) Output() iotago.Output {
	return m.output
}

func (m *MockedState) CreationTime() iotago.SlotIndex {
	return m.creationTime
}

func (m *MockedState) String() string {
	return "MockedOutput(" + m.id.ToHex() + ")"
}
