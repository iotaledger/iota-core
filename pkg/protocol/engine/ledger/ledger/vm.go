package ledger

import (
	"context"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	iotago "github.com/iotaledger/iota.go/v4"
	iotagovm "github.com/iotaledger/iota.go/v4/vm"
	"github.com/iotaledger/iota.go/v4/vm/stardust"
)

func (l *Ledger) executeStardustVM(_ context.Context, stateTransition mempool.Transaction, inputStates []mempool.OutputState, timeReference mempool.ContextState) ([]mempool.OutputState, error) {
	tx, ok := stateTransition.(*iotago.Transaction)
	if !ok {
		return nil, iotago.ErrTxTypeInvalid
	}

	inputSet := iotagovm.InputSet{}
	for _, inputState := range inputStates {
		inputSet[inputState.OutputID()] = iotagovm.OutputWithCreationSlot{
			Output:       inputState.Output(),
			CreationSlot: inputState.SlotCreated(),
		}
	}
	resolvedInputs := iotagovm.ResolvedInputs{
		InputSet: inputSet,
	}

	bicInputs, err := tx.BICInputs()
	if err != nil {
		return nil, ierrors.Join(err, iotago.ErrBICInputInvalid)
	}

	rewardInputs, err := tx.RewardInputs()
	if err != nil {
		return nil, ierrors.Join(err, iotago.ErrRewardInputInvalid)
	}

	commitmentInput, ok := timeReference.(*iotago.Commitment)
	if commitmentInput != nil && !ok {
		return nil, ierrors.Join(iotago.ErrCommitmentInputInvalid, ierrors.New("unsupported type for time reference"))
	}

	resolvedInputs.CommitmentInput = commitmentInput

	if (len(rewardInputs) > 0 || len(bicInputs) > 0) && commitmentInput == nil {
		return nil, iotago.ErrCommitmentInputMissing
	}

	bicInputSet := make(iotagovm.BlockIssuanceCreditInputSet)
	for _, inp := range bicInputs {
		accountData, exists, accountErr := l.accountsLedger.Account(inp.AccountID, commitmentInput.Index)
		if accountErr != nil {
			return nil, ierrors.Join(iotago.ErrBICInputInvalid, ierrors.Wrapf(accountErr, "could not get BIC input for account %s in slot %d", inp.AccountID, commitmentInput.Index))
		}
		if !exists {
			return nil, ierrors.Join(iotago.ErrBICInputInvalid, ierrors.Errorf("BIC input does not exist for account %s in slot %d", inp.AccountID, commitmentInput.Index))
		}

		bicInputSet[inp.AccountID] = accountData.Credits.Value
	}
	resolvedInputs.BlockIssuanceCreditInputSet = bicInputSet

	rewardInputSet := make(iotagovm.RewardsInputSet)
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

			reward, _, _, rewardErr := l.sybilProtection.ValidatorReward(accountID, stakingFeature.StakedAmount, stakingFeature.StartEpoch, stakingFeature.EndEpoch)
			if rewardErr != nil {
				return nil, ierrors.Wrapf(iotago.ErrFailedToClaimStakingReward, "failed to get Validator reward for AccountOutput %s at index %d (StakedAmount: %d, StartEpoch: %d, EndEpoch: %d", outputID, inp.Index, stakingFeature.StakedAmount, stakingFeature.StartEpoch, stakingFeature.EndEpoch)
			}

			rewardInputSet[accountID] = reward

		case *iotago.DelegationOutput:
			delegationID := castOutput.DelegationID
			if delegationID.Empty() {
				delegationID = iotago.DelegationIDFromOutputID(outputID)
			}

			delegationEnd := castOutput.EndEpoch
			if delegationEnd == 0 {
				delegationEnd = l.apiProvider.APIForSlot(commitmentInput.Index).TimeProvider().EpochFromSlot(commitmentInput.Index) - iotago.EpochIndex(1)
			}

			reward, _, _, rewardErr := l.sybilProtection.DelegatorReward(castOutput.ValidatorAddress.AccountID(), castOutput.DelegatedAmount, castOutput.StartEpoch, delegationEnd)
			if rewardErr != nil {
				return nil, ierrors.Wrapf(iotago.ErrFailedToClaimDelegationReward, "failed to get Delegator reward for DelegationOutput %s at index %d (StakedAmount: %d, StartEpoch: %d, EndEpoch: %d", outputID, inp.Index, castOutput.DelegatedAmount, castOutput.StartEpoch, castOutput.EndEpoch)
			}

			rewardInputSet[delegationID] = reward
		}
	}
	resolvedInputs.RewardsInputSet = rewardInputSet

	api := l.apiProvider.APIForSlot(tx.Essence.CreationSlot)

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

	created := make([]mempool.OutputState, 0, len(outputSet))
	for outputID, output := range outputSet {
		created = append(created, &ExecutionOutput{
			outputID:     outputID,
			output:       output,
			creationSlot: tx.Essence.CreationSlot,
		})
	}

	return created, nil
}
