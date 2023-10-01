package ledger

import (
	"context"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	iotago "github.com/iotaledger/iota.go/v4"
	iotagovm "github.com/iotaledger/iota.go/v4/vm"
	"github.com/iotaledger/iota.go/v4/vm/stardust"
)

func (l *Ledger) executeStardustVM(ctx context.Context, stateTransition mempool.Transaction, inputStates []mempool.OutputState, timeReference mempool.ContextState) ([]mempool.OutputState, error) {
	tx, ok := stateTransition.(*iotago.SignedTransaction)
	if !ok {
		return nil, iotago.ErrTxTypeInvalid
	}

	inputSet := iotagovm.InputSet{}
	for _, input := range inputStates {
		inputSet[input.OutputID()] = input.Output()
	}

	commitmentInput, ok := timeReference.(*iotago.Commitment)
	if commitmentInput != nil && !ok {
		return nil, ierrors.Join(iotago.ErrCommitmentInputInvalid, ierrors.New("unsupported type for time reference"))
	}

	if err := l.validateStardustVMTransaction(ctx, tx, inputSet, commitmentInput); err != nil {
		return nil, err
	}

	outputSet, err := tx.OutputsSet()
	if err != nil {
		return nil, err
	}

	created := make([]mempool.OutputState, 0, len(outputSet))
	for outputID, output := range outputSet {
		created = append(created, &ExecutionOutput{
			outputID:     outputID,
			output:       output,
			creationSlot: tx.Transaction.CreationSlot,
		})
	}

	return created, nil
}

func (l *Ledger) validateStardustVMTransaction(_ context.Context, tx *iotago.SignedTransaction, inputSet iotagovm.InputSet, commitmentInput *iotago.Commitment) error {
	resolvedInputs := iotagovm.ResolvedInputs{
		InputSet: inputSet,
	}

	bicInputs, err := tx.BICInputs()
	if err != nil {
		return ierrors.Join(err, iotago.ErrBICInputInvalid)
	}

	rewardInputs, err := tx.RewardInputs()
	if err != nil {
		return ierrors.Join(err, iotago.ErrRewardInputInvalid)
	}

	resolvedInputs.CommitmentInput = commitmentInput

	if (len(rewardInputs) > 0 || len(bicInputs) > 0) && commitmentInput == nil {
		return iotago.ErrCommitmentInputMissing
	}

	bicInputSet := make(iotagovm.BlockIssuanceCreditInputSet)
	for _, inp := range bicInputs {
		accountData, exists, accountErr := l.accountsLedger.Account(inp.AccountID, commitmentInput.Slot)
		if accountErr != nil {
			return ierrors.Join(iotago.ErrBICInputInvalid, ierrors.Wrapf(accountErr, "could not get BIC input for account %s in slot %d", inp.AccountID, commitmentInput.Slot))
		}
		if !exists {
			return ierrors.Join(iotago.ErrBICInputInvalid, ierrors.Errorf("BIC input does not exist for account %s in slot %d", inp.AccountID, commitmentInput.Slot))
		}

		bicInputSet[inp.AccountID] = accountData.Credits.Value
	}
	resolvedInputs.BlockIssuanceCreditInputSet = bicInputSet

	rewardInputSet := make(iotagovm.RewardsInputSet)
	for _, inp := range rewardInputs {
		outputID := tx.Transaction.Inputs[inp.Index].(*iotago.UTXOInput).OutputID()

		switch castOutput := inputSet[outputID].(type) {
		case *iotago.AccountOutput:
			stakingFeature := castOutput.FeatureSet().Staking()
			if stakingFeature == nil {
				return ierrors.Wrapf(iotago.ErrNoStakingFeature, "cannot claim rewards from an AccountOutput %s at index %d without staking feature", outputID, inp.Index)
			}
			accountID := castOutput.AccountID
			if accountID.Empty() {
				accountID = iotago.AccountIDFromOutputID(outputID)
			}

			reward, _, _, rewardErr := l.sybilProtection.ValidatorReward(accountID, stakingFeature.StakedAmount, stakingFeature.StartEpoch, stakingFeature.EndEpoch)
			if rewardErr != nil {
				return ierrors.Wrapf(iotago.ErrFailedToClaimStakingReward, "failed to get Validator reward for AccountOutput %s at index %d (StakedAmount: %d, StartEpoch: %d, EndEpoch: %d", outputID, inp.Index, stakingFeature.StakedAmount, stakingFeature.StartEpoch, stakingFeature.EndEpoch)
			}

			rewardInputSet[accountID] = reward

		case *iotago.DelegationOutput:
			delegationID := castOutput.DelegationID
			if delegationID.Empty() {
				delegationID = iotago.DelegationIDFromOutputID(outputID)
			}

			delegationEnd := castOutput.EndEpoch
			if delegationEnd == 0 {
				delegationEnd = l.apiProvider.APIForSlot(commitmentInput.Slot).TimeProvider().EpochFromSlot(commitmentInput.Slot) - iotago.EpochIndex(1)
			}

			reward, _, _, rewardErr := l.sybilProtection.DelegatorReward(castOutput.ValidatorAddress.AccountID(), castOutput.DelegatedAmount, castOutput.StartEpoch, delegationEnd)
			if rewardErr != nil {
				return ierrors.Wrapf(iotago.ErrFailedToClaimDelegationReward, "failed to get Delegator reward for DelegationOutput %s at index %d (StakedAmount: %d, StartEpoch: %d, EndEpoch: %d", outputID, inp.Index, castOutput.DelegatedAmount, castOutput.StartEpoch, castOutput.EndEpoch)
			}

			rewardInputSet[delegationID] = reward
		}
	}
	resolvedInputs.RewardsInputSet = rewardInputSet

	vmParams := &iotagovm.Params{
		API: tx.API,
	}

	return stardust.NewVirtualMachine().Execute(tx, vmParams, resolvedInputs)
}

func (l *Ledger) ValidateTransactionInVM(ctx context.Context, transaction *iotago.SignedTransaction) error {
	inputSet := iotagovm.InputSet{}
	inputs, err := transaction.Inputs()
	if err != nil {
		return ierrors.Wrapf(err, "failed to get inputs for transaction")
	}

	for _, input := range inputs {
		output, spent, err := l.OutputOrSpent(input.OutputID())
		if err != nil {
			return ierrors.Wrapf(err, "failed to get output for input %s", input.OutputID())
		}
		if spent != nil {
			return ierrors.Errorf("output %s is already spent", input.OutputID())
		}

		inputSet[output.OutputID()] = output.Output()
	}

	contextInputs, err := transaction.ContextInputs()
	if err != nil {
		return ierrors.Wrapf(err, "failed to get context inputs for transaction")
	}

	var commitment *iotago.Commitment
	for _, contextInput := range contextInputs {
		if commitmentInput, ok := contextInput.(*iotago.CommitmentInput); ok {
			commitment, err = l.loadCommitment(commitmentInput.CommitmentID)
			if err != nil {
				return ierrors.Wrapf(err, "failed to load commitment %s", commitmentInput.CommitmentID)
			}
			break
		}
	}

	return l.validateStardustVMTransaction(ctx, transaction, inputSet, commitment)
}
