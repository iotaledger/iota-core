package utxoledger

import (
	"context"

	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	iotago "github.com/iotaledger/iota.go/v4"
	iotagovm "github.com/iotaledger/iota.go/v4/vm"
	"github.com/iotaledger/iota.go/v4/vm/stardust"
)

func executeStardustVM(_ context.Context, stateTransition mempool.Transaction, inputStates []mempool.State) (outputStates []mempool.State, err error) {
	tx, ok := stateTransition.(*iotago.Transaction)
	if !ok {
		return nil, ErrUnexpectedUnderlyingType
	}

	txCreationTime := tx.Essence.CreationTime

	inputSet := iotago.OutputSet{}
	for _, inputState := range inputStates {
		inputSet[inputState.OutputID()] = inputState.Output()
	}

	params := &iotagovm.Params{
		External: &iotago.ExternalUnlockParameters{
			//TODO: remove this workaround after the VM gets adapted to use the tx creationtime
			ConfUnix: uint32(txCreationTime.Unix()),
		},
	}

	if err := stardust.NewVirtualMachine().Execute(tx, params, inputSet); err != nil {
		return nil, err
	}

	outputSet, err := tx.OutputsSet()
	if err != nil {
		return nil, err
	}

	created := make([]mempool.State, len(outputSet))
	for outputID, output := range outputSet {
		created = append(created, &ExecutionOutput{
			outputID: outputID,
			output:   output,
		})
	}

	return created, nil
}
