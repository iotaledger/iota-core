package ledger

import (
	"context"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	iotago "github.com/iotaledger/iota.go/v4"
	iotagovm "github.com/iotaledger/iota.go/v4/vm"
	"github.com/iotaledger/iota.go/v4/vm/stardust"
)

func (l *Ledger) executeStardustVM(_ context.Context, stateTransition mempool.Transaction, inputStates []mempool.State) ([]mempool.State, error) {
	tx, ok := stateTransition.(*iotago.Transaction)
	if !ok {
		return nil, iotago.ErrUnknownTransactinType
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
		return nil, ierrors.Join(err, iotago.ErrCouldNotResolveBICInput)
	}

	rewardInputs, err := tx.RewardInputs()
	if err != nil {
		return nil, ierrors.Join(err, iotago.ErrCouldNotResolveRewardInput)
	}

	// resolve the commitment inputs from storage
	commitment := tx.CommitmentInput()

	if (len(rewardInputs) > 0 || len(bicInputs) > 0) && commitment == nil {
		return nil, iotago.ErrCommittmentInputMissing
	}

	var loadedCommitment *iotago.Commitment
	if commitment != nil {
		loadedCommitment, err = l.loadCommitment(commitment.CommitmentID)
		if err != nil {
			return nil, ierrors.Join(iotago.ErrCouldNorRetrieveCommitment, ierrors.Wrapf(err, "could not load commitment %s", commitment.CommitmentID))
		}

		resolvedInputs.CommitmentInput = loadedCommitment
	}

	bicInputSet := make(iotagovm.BlockIssuanceCreditInputSet)
	for _, inp := range bicInputs {
		accountData, exists, accountErr := l.accountsLedger.Account(inp.AccountID, loadedCommitment.Index)
		if accountErr != nil {
			return nil, ierrors.Join(iotago.ErrCouldNotResolveBICInput, ierrors.Wrapf(accountErr, "could not get BIC input for account %s in slot %d", inp.AccountID, loadedCommitment.Index))
		}
		if !exists {
			return nil, ierrors.Join(iotago.ErrCouldNotResolveBICInput, ierrors.Errorf("BIC input does not exist for account %s in slot %d", inp.AccountID, loadedCommitment.Index))
		}

		bicInputSet[inp.AccountID] = accountData.Credits.Value
	}
	resolvedInputs.BlockIssuanceCreditInputSet = bicInputSet

	rewardInputSet := make(iotagovm.RewardsInputSet)
	// TODO: when refactoring this to be resolved by the mempool, the function that resolves ContextInputs should
	// receive a slice with promises of UTXO inputs and wait until the necessary inputs are available
	for _, inp := range rewardInputs {
		outputID := inputStates[inp.Index].OutputID()

		switch castOutput := inputStates[inp.Index].Output().(type) {
		case *iotago.AccountOutput:
			stakingFeature := castOutput.FeatureSet().Staking()
			if stakingFeature == nil {
				return nil, ierrors.Wrapf(iotago.ErrNoStakingFeature, "cannot claim rewards from an AccountOutput %s at index %d without staking feature", outputID, inp.Index)
			}
			accountID := castOutput.AccountID
			if accountID.Empty() {
				accountID = iotago.AccountIDFromOutputID(outputID)
			}

			reward, rewardErr := l.sybilProtection.ValidatorReward(accountID, stakingFeature.StakedAmount, stakingFeature.StartEpoch, stakingFeature.EndEpoch)
			if rewardErr != nil {
				return nil, ierrors.Wrapf(iotago.ErrFailedToClaimValidatorReward, "failed to get Validator reward for AccountOutput %s at index %d (StakedAmount: %d, StartEpoch: %d, EndEpoch: %d", outputID, inp.Index, stakingFeature.StakedAmount, stakingFeature.StartEpoch, stakingFeature.EndEpoch)
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

			reward, rewardErr := l.sybilProtection.DelegatorReward(castOutput.ValidatorID, castOutput.DelegatedAmount, castOutput.StartEpoch, delegationEnd)
			if rewardErr != nil {
				return nil, ierrors.Wrapf(iotago.ErrFailedToClaimDelegatorReward, "failed to get Delegator reward for DelegationOutput %s at index %d (StakedAmount: %d, StartEpoch: %d, EndEpoch: %d", outputID, inp.Index, castOutput.DelegatedAmount, castOutput.StartEpoch, castOutput.EndEpoch)
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
		return nil, ierrors.Errorf("could not get commitment inputs: %w", err)
	}
	storedCommitmentID, err := c.Commitment().ID()
	if err != nil {
		return nil, ierrors.Errorf("could compute commitment ID: %w", err)
	}
	if storedCommitmentID != inputCommitmentID {
		return nil, ierrors.Errorf("commitment ID of input %s different to stored commitment %s", inputCommitmentID, storedCommitmentID)
	}

	return c.Commitment(), nil
}
