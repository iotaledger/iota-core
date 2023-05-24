package mempool

import iotago "github.com/iotaledger/iota.go/v4"

type State interface {
	// OutputID returns the identifier of the State.
	OutputID() iotago.OutputID

	//Output returns the underlying Output of the State.
	Output() iotago.Output
}
