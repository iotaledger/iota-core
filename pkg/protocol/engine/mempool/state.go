package mempool

import iotago "github.com/iotaledger/iota.go/v4"

type State interface {
	StateID() iotago.Identifier

	Type() iotago.StateType
}

type OutputState interface {
	State
	// OutputID returns the identifier of the State.
	OutputID() iotago.OutputID

	// Output returns the underlying Output of the State.
	Output() iotago.Output

	// CreationSlot returns the slot when the State was created.
	CreationSlot() iotago.SlotIndex
}

type ContextState interface {
	State
}
