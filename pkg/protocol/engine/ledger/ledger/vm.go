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
		return nil, xerrors.Errorf("could not get Reward inputs: %w", err)
	}

	// resolve the commitment inputs from storage
	commitment := tx.CommitmentInput()

	if (len(rewardInputs) > 0 || len(bicInputs) > 0) && commitment == nil {
		return nil, xerrors.Errorf("commitment input required with Reward or BIC input")
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
		b, _, accountErr := l.accountsLedger.Account(inp.AccountID, loadedCommitment.Index)
		if accountErr != nil {
			return nil, xerrors.Errorf("could not get BIC inputs: %w", accountErr)
		}

		bicInputSet[inp.AccountID] = b.Credits.Value
	}
	resolvedInputs.BICInputSet = bicInputSet

	rewardInputSet := make(iotagovm.RewardsInputSet)
	// when refactoring this to be resolved by the mempool, the function that resolves ContextInputs should receive a slice with promises of UTXO inputs and wait until the necessary
	for _, inp := range rewardInputs {
		switch castOutput := inputStates[inp.Index].Output().(type) {
		case *iotago.AccountOutput:
			stakingFeature := castOutput.FeatureSet().Staking()
			if stakingFeature == nil {
				return nil, xerrors.Errorf("cannot claim rewards from an AccountOutput at index %d without staking feature", inp.Index)
			}
			reward, rewardErr := l.rewardsManager.ValidatorReward(castOutput.AccountID, stakingFeature.StakedAmount, stakingFeature.StartEpoch, stakingFeature.EndEpoch)
			if rewardErr != nil {
				return nil, xerrors.Errorf("failed to get Validator reward for AccountOutput at index %d (StakedAmount: %d, StartEpoch: %d, EndEpoch: %d", castOutput.AccountID, stakingFeature.StakedAmount, stakingFeature.StartEpoch, stakingFeature.EndEpoch)
			}

			rewardInputSet[castOutput.AccountID] += reward
		case *iotago.DelegationOutput:
			reward, rewardErr := l.rewardsManager.DelegatorReward(castOutput.ValidatorID, castOutput.DelegatedAmount, castOutput.StartEpoch, castOutput.EndEpoch)
			if rewardErr != nil {
				return nil, xerrors.Errorf("failed to get Delegator reward for DelegationOutput at index %d (StakedAmount: %d, StartEpoch: %d, EndEpoch: %d", castOutput.DelegationID, castOutput.DelegatedAmount, castOutput.StartEpoch, castOutput.EndEpoch)
			}

			rewardInputSet[castOutput.DelegationID] += reward
		}
	}
	resolvedInputs.RewardsInputSet = rewardInputSet

	vmParams := &iotagovm.Params{External: &iotago.ExternalUnlockParameters{
		ProtocolParameters: l.protocolParameters,
	}}
	if err = stardust.NewVirtualMachine().Execute(tx, vmParams, resolvedInputs); err != nil {
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
