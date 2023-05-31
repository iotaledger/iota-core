package utxoledger

import (
	"context"

	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	iotago "github.com/iotaledger/iota.go/v4"
	iotagovm "github.com/iotaledger/iota.go/v4/vm"
	"github.com/iotaledger/iota.go/v4/vm/stardust"
	"golang.org/x/xerrors"
)

func (l *Ledger) executeStardustVM(_ context.Context, stateTransition mempool.Transaction, inputStates []mempool.State) (outputStates []mempool.State, err error) {
	tx, ok := stateTransition.(*iotago.Transaction)
	if !ok {
		return nil, ErrUnexpectedUnderlyingType
	}

	inputSet := iotago.InputSet{}
	for _, inputState := range inputStates {
		inputSet[inputState.OutputID()] = iotago.OutputWithCreationTime{
			Output:       inputState.Output(),
			CreationTime: inputState.CreationTime(),
		}
	}

	bicInputSet := iotago.BICInputSet{}
	bicInputs, err := tx.BICInputs()
	if err != nil {
		return nil, xerrors.Errorf("could not get BIC inputs: %w", err)
	}
	for _, inp := range bicInputs {
		// get the BIC inputs from bic manager
		b, err := l.accountsLedger.BIC(inp.AccountID, inp.CommitmentID.Index())
		if err != nil {
			return nil, xerrors.Errorf("could not get BIC inputs: %w", err)
		}
		bicInputSet[inp.AccountID] = iotago.BlockIssuanceCredit{
			AccountID:    inp.AccountID,
			CommitmentID: inp.CommitmentID,
			Value:        b.Credits().Value,
		}

	}

	// TODO: get Commitment inputs from storage
	// l.commitmentLoader(input.CommitmentID.Index())

	resolvedInputs := iotago.ResolvedInputs{
		InputSet:    inputSet,
		BICInputSet: bicInputSet,
	}

	if err := stardust.NewVirtualMachine().Execute(tx, &iotagovm.Params{}, resolvedInputs); err != nil {
		return nil, err
	}

	outputSet, err := tx.OutputsSet()
	if err != nil {
		return nil, err
	}

	created := make([]mempool.State, 0, len(outputSet))
	for outputID, output := range outputSet {
		created = append(created, &ExecutionOutput{
			outputID:     outputID,
			output:       output,
			creationTime: tx.Essence.CreationTime,
		})
	}

	return created, nil
}
