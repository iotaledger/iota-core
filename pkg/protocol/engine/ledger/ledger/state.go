package ledger

import (
	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
)

type ExecutionOutput struct {
	outputID     iotago.OutputID
	output       iotago.Output
	creationTime iotago.SlotIndex
}

func (o *ExecutionOutput) StateID() iotago.Identifier {
	return iotago.IdentifierFromData(lo.PanicOnErr(o.outputID.Bytes()))
}

func (o *ExecutionOutput) Type() iotago.StateType {
	return iotago.InputUTXO
}

func (s *ExecutionOutput) OutputID() iotago.OutputID {
	return s.outputID
}

func (s *ExecutionOutput) Output() iotago.Output {
	return s.output
}

func (s *ExecutionOutput) CreationTime() iotago.SlotIndex {
	return s.creationTime
}
