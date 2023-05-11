package utxoledger

import (
	iotago "github.com/iotaledger/iota.go/v4"
)

type State struct {
	outputID iotago.OutputID
	output   iotago.Output
}

func (s *State) ID() iotago.OutputID {
	return s.outputID
}
