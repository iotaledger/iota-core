package ledger

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	iotago "github.com/iotaledger/iota.go/v4"
	iotagovm "github.com/iotaledger/iota.go/v4/vm"
	"github.com/iotaledger/iota.go/v4/vm/stardust"
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

	// resolve the BIC inputs from the BIC Manager
	bicInputSet := iotago.BICInputSet{}
	bicInputs, err := tx.BICInputs()
	if err != nil {
		return nil, xerrors.Errorf("could not get BIC inputs: %w", err)
	}
	for _, inp := range bicInputs {
		// get the BIC inputs from bic manager
		if _, err := l.loadCommitment(inp.CommitmentID); err != nil {
			return nil, xerrors.Errorf("could not load commitment: %w", err)
		}
		b, _, err := l.accountsLedger.Account(inp.AccountID, inp.CommitmentID.Index())
		if err != nil {
			return nil, xerrors.Errorf("could not get BIC inputs: %w", err)
		}
		bicInputSet[inp.AccountID] = iotago.BlockIssuanceCredit{
			AccountID:    inp.AccountID,
			CommitmentID: inp.CommitmentID,
			Value:        b.Credits.Value,
		}
	}

	// resolve the commitment inputs from storage
	commitmentInputSet := iotago.CommitmentInputSet{}
	commitmentInputs, err := tx.CommitmentInputs()
	if err != nil {
		return nil, xerrors.Errorf("could not get Commitment inputs: %w", err)
	}
	for _, inp := range commitmentInputs {
		c, err := l.loadCommitment(inp.CommitmentID)
		if err != nil {
			return nil, xerrors.Errorf("could not load commitment: %w", err)
		}
		commitmentInputSet[inp.CommitmentID] = c
	}

	resolvedInputs := iotago.ResolvedInputs{
		InputSet:           inputSet,
		BICInputSet:        bicInputSet,
		CommitmentInputSet: commitmentInputSet,
	}

	vmParams := &iotagovm.Params{External: &iotago.ExternalUnlockParameters{
		ProtocolParameters: l.protocolParameters,
	}}
	if err := stardust.NewVirtualMachine().Execute(tx, vmParams, resolvedInputs); err != nil {
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

func (l *Ledger) loadCommitment(inputCommitmentID iotago.CommitmentID) (*iotago.Commitment, error) {
	// TODO: cache the loaded commitments
	c, err := l.commitmentLoader(inputCommitmentID.Index())
	if err != nil {
		return nil, xerrors.Errorf("could not get commitment inputs: %w", err)
	}
	storedCommitmentID, err := c.Commitment().ID()
	if err != nil {
		return nil, xerrors.Errorf("could compute commitment ID: %w", err)
	}
	if storedCommitmentID != inputCommitmentID {
		return nil, xerrors.Errorf("commitment ID of input %s different to stored commitment %s", inputCommitmentID, storedCommitmentID)
	}
	return c.Commitment(), nil
}
