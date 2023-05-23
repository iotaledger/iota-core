package utxoledger

import (
	iotago "github.com/iotaledger/iota.go/v4"
)

type ExecutionOutput struct {
	outputID iotago.OutputID
	output   iotago.Output
}

func (s *ExecutionOutput) OutputID() iotago.OutputID {
	return s.outputID
}

func (s *ExecutionOutput) Output() iotago.Output {
	return s.output
}
