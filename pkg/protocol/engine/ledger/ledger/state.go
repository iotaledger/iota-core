package ledger

import (
	iotago "github.com/iotaledger/iota.go/v4"
)

type ExecutionOutput struct {
	outputID     iotago.OutputID
	output       iotago.Output
	creationTime iotago.SlotIndex
}

func (s *ExecutionOutput) OutputID() iotago.OutputID {
	return s.outputID
}

func (s *ExecutionOutput) Output() iotago.Output {
	return s.output
}

func (s *ExecutionOutput) CreationTime() iotago.SlotIndex
