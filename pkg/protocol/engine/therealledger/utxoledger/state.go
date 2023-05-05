package utxoledger

import (
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledgerstate"
	iotago "github.com/iotaledger/iota.go/v4"
)

type State struct {
	outputID iotago.OutputID
	output   iotago.Output

	utxoOutput *ledgerstate.Output
}

func (s *State) ID() iotago.OutputID {
	if s.utxoOutput != nil {
		return s.utxoOutput.OutputID()
	}
	return s.outputID
}
