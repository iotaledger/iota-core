package ledger

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	iotago "github.com/iotaledger/iota.go/v4"
	iotagovm "github.com/iotaledger/iota.go/v4/vm"
	"github.com/iotaledger/iota.go/v4/vm/stardust"
)

func (l *Ledger) executeStardustVM(_ context.Context, stateTransition mempool.Transaction, inputStates []mempool.State) ([]mempool.State, error) {
	tx, ok := stateTransition.(*iotago.Transaction)
	if !ok {
		return nil, ErrUnexpectedUnderlyingType
	}

	inputSet := iotagovm.InputSet{}
	for _, inputState := range inputStates {
		inputSet[inputState.OutputID()] = iotagovm.OutputWithCreationTime{
			Output:       inputState.Output(),
			CreationTime: inputState.CreationTime(),
		}
	}
	resolvedInputs := iotagovm.ResolvedInputs{
		InputSet: inputSet,
	}

	// TODO: refactor resolution of ContextInputs to be handled by Mempool
	bicInputs, err := tx.BICInputs()
	if err != nil {
		return nil, xerrors.Errorf("could not get BIC inputs: %w", err)
	}

	rewardInputs, err := tx.RewardInputs()
	if err != nil {
		return nil, xerrors.Errorf("could not get reward inputs: %w", err)
	}

	// resolve the commitment inputs from storage
	commitment := tx.CommitmentInput()

	if (len(rewardInputs) > 0 || len(bicInputs) > 0) && commitment == nil {
		return nil, xerrors.Errorf("commitment input required with reward or BIC input")
	}

	var loadedCommitment *iotago.Commitment
	if commitment != nil {
		loadedCommitment, err = l.loadCommitment(commitment.CommitmentID)
		if err != nil {
			return nil, xerrors.Errorf("could not load commitment: %w", err)
		}

		resolvedInputs.CommitmentInput = loadedCommitment
	}

	bicInputSet := make(iotagovm.BICInputSet)
	for _, inp := range bicInputs {
		accountData, exists, accountErr := l.accountsLedger.Account(inp.AccountID, loadedCommitment.Index)
		if accountErr != nil {
			return nil, xerrors.Errorf("could not get BIC input for account %s in slot %d: %w", inp.AccountID, loadedCommitment.Index, accountErr)
		}
		if !exists {
			return nil, xerrors.Errorf("BIC input does not exist for account %s in slot %d", inp.AccountID, loadedCommitment.Index)
		}

		bicInputSet[inp.AccountID] = accountData.Credits.Value
	}
	resolvedInputs.BICInputSet = bicInputSet

	rewardInputSet := make(iotagovm.RewardsInputSet)
	// when refactoring this to be resolved by the mempool, the function that resolves ContextInputs should receive a slice with promises of UTXO inputs and wait until the necessary
	for _, inp := range rewardInputs {
		outputID := inputStates[inp.Index].OutputID()

		switch castOutput := inputStates[inp.Index].Output().(type) {
		case *iotago.AccountOutput:
			stakingFeature := castOutput.FeatureSet().Staking()
			if stakingFeature == nil {
				return nil, xerrors.Errorf("cannot claim rewards from an AccountOutput %s at index %d without staking feature", outputID, inp.Index)
			}
			accountID := castOutput.AccountID
			if accountID.Empty() {
				accountID = iotago.AccountIDFromOutputID(outputID)
			}

			reward, rewardErr := l.epochGadget.ValidatorReward(accountID, stakingFeature.StakedAmount, stakingFeature.StartEpoch, stakingFeature.EndEpoch)
			if rewardErr != nil {
				return nil, xerrors.Errorf("failed to get Validator reward for AccountOutput %s at index %d (StakedAmount: %d, StartEpoch: %d, EndEpoch: %d", outputID, inp.Index, stakingFeature.StakedAmount, stakingFeature.StartEpoch, stakingFeature.EndEpoch)
			}

			rewardInputSet[accountID] = reward

		case *iotago.DelegationOutput:
			delegationID := castOutput.DelegationID
			if delegationID.Empty() {
				delegationID = iotago.DelegationIDFromOutputID(outputID)
			}

			delegationEnd := castOutput.EndEpoch
			if delegationEnd == 0 {
				delegationEnd = l.apiProvider.APIForSlot(loadedCommitment.Index).TimeProvider().EpochFromSlot(loadedCommitment.Index) - iotago.EpochIndex(1)
			}

			reward, rewardErr := l.epochGadget.DelegatorReward(castOutput.ValidatorID, castOutput.DelegatedAmount, castOutput.StartEpoch, delegationEnd)
			if rewardErr != nil {
				return nil, xerrors.Errorf("failed to get Delegator reward for DelegationOutput %s at index %d (StakedAmount: %d, StartEpoch: %d, EndEpoch: %d", outputID, inp.Index, castOutput.DelegatedAmount, castOutput.StartEpoch, castOutput.EndEpoch)
			}

			rewardInputSet[delegationID] = reward
		}
	}
	resolvedInputs.RewardsInputSet = rewardInputSet

	//TODO: in which slot is this transaction?
	api := l.apiProvider.APIForSlot(tx.Essence.CreationTime)

	vmParams := &iotagovm.Params{
		API: api,
	}
	if err = stardust.NewVirtualMachine().Execute(tx, vmParams, resolvedInputs); err != nil {
		return nil, err
	}

	outputSet, err := tx.OutputsSet(api)
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
