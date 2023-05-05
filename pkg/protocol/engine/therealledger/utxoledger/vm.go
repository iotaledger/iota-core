package utxoledger

import (
	"context"

	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledger"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	iotago "github.com/iotaledger/iota.go/v4"
	iotagovm "github.com/iotaledger/iota.go/v4/vm"
	"github.com/iotaledger/iota.go/v4/vm/stardust"
)

func (l *Ledger) executeStardustVM(stateTransition mempool.Transaction, inputs []ledger.State, ctx context.Context) (outputs []ledger.State, err error) {
	tx := stateTransition.(*Transaction)
	txCreationTime := tx.Transaction.Essence.CreationTime

	inputSet := iotago.OutputSet{}
	for _, input := range inputs {
		s := input.(*State)
		inputSet[s.outputID] = s.output
	}

	params := &iotagovm.Params{
		External: &iotago.ExternalUnlockParameters{
			//TODO: remove this workaround after the VM gets adapted to use the tx creationtime
			ConfUnix: uint32(txCreationTime.Unix()),
		},
	}

	if err := stardust.NewVirtualMachine().Execute(tx.Transaction, params, inputSet); err != nil {
		return nil, err
	}

	outputSet, err := tx.Transaction.OutputsSet()
	if err != nil {
		return nil, err
	}

	var created []ledger.State
	for outputID, output := range outputSet {
		created = append(created, &State{
			outputID: outputID,
			output:   output,
		})
	}

	return created, nil
}
