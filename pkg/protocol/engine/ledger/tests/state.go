package ledgertests

import iotago "github.com/iotaledger/iota.go/v4"

type State struct {
	id iotago.OutputID
}

func NewState(transactionID iotago.TransactionID, index uint16) *State {
	return &State{id: iotago.OutputIDFromTransactionIDAndIndex(transactionID, index)}
}

func (m *State) ID() iotago.OutputID {
	return m.id
}

func (m *State) String() string {
	return "MockedOutput(" + m.id.ToHex() + ")"
}
