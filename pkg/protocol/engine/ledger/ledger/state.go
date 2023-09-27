package ledger

import (
	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
)

type ExecutionOutput struct {
	outputID     iotago.OutputID
	output       iotago.Output
	creationSlot iotago.SlotIndex
}

func (o *ExecutionOutput) StateID() iotago.Identifier {
	return iotago.IdentifierFromData(lo.PanicOnErr(o.outputID.Bytes()))
}

func (o *ExecutionOutput) Type() iotago.StateType {
	return iotago.InputUTXO
}

func (o *ExecutionOutput) OutputID() iotago.OutputID {
	return o.outputID
}

func (o *ExecutionOutput) Output() iotago.Output {
	return o.output
}

func (o *ExecutionOutput) SlotCreated() iotago.SlotIndex {
	return o.creationSlot
}
