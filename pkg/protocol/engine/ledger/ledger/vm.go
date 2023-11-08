package ledger

import (
	"context"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger"
	iotago "github.com/iotaledger/iota.go/v4"
	iotagovm "github.com/iotaledger/iota.go/v4/vm"
	"github.com/iotaledger/iota.go/v4/vm/nova"
)

type VM struct {
	ledger *Ledger
}

func NewVM(ledger *Ledger) *VM {
	return &VM{
		ledger: ledger,
	}
}

func (v *VM) Inputs(transaction mempool.Transaction) (inputReferences []mempool.StateReference, err error) {
	stardustTransaction, ok := transaction.(*iotago.Transaction)
	if !ok {
		return nil, iotago.ErrTxTypeInvalid
	}

	for _, input := range stardustTransaction.TransactionEssence.Inputs {
		inputReferences = append(inputReferences, input)
	}
	for _, input := range stardustTransaction.TransactionEssence.ContextInputs {
		inputReferences = append(inputReferences, input)
	}

	return inputReferences, nil
}

func (v *VM) ValidateSignatures(signedTransaction mempool.SignedTransaction, resolvedInputStates []mempool.State) (executionContext context.Context, err error) {
	signedStardustTransaction, ok := signedTransaction.(*iotago.SignedTransaction)
	if !ok {
		return nil, iotago.ErrTxTypeInvalid
	}

	utxoInputSet := iotagovm.InputSet{}
	commitmentInput := (*iotago.Commitment)(nil)
	bicInputs := make([]*iotago.BlockIssuanceCreditInput, 0)
	rewardInputs := make([]*iotago.RewardInput, 0)
	for _, resolvedInput := range resolvedInputStates {
		resolvedInput.Type()
		switch typedInput := resolvedInput.(type) {
		case *iotago.Commitment:
			commitmentInput = typedInput
		case *iotago.BlockIssuanceCreditInput:
			bicInputs = append(bicInputs, typedInput)
		case *iotago.RewardInput:
			rewardInputs = append(rewardInputs, typedInput)
		case *utxoledger.Output:
			utxoInputSet[typedInput.OutputID()] = typedInput.Output()
		}
	}

	if (len(rewardInputs) > 0 || len(bicInputs) > 0) && commitmentInput == nil {
		return nil, iotago.ErrCommitmentInputMissing
	}

	bicInputSet := make(iotagovm.BlockIssuanceCreditInputSet)
	for _, inp := range bicInputs {
		accountData, exists, accountErr := v.ledger.accountsLedger.Account(inp.AccountID, commitmentInput.Slot)
		if accountErr != nil {
			return nil, ierrors.Join(iotago.ErrBICInputInvalid, ierrors.Wrapf(accountErr, "could not get BIC input for account %s in slot %d", inp.AccountID, commitmentInput.Slot))
		}
		if !exists {
			return nil, ierrors.Join(iotago.ErrBICInputInvalid, ierrors.Errorf("BIC input does not exist for account %s in slot %d", inp.AccountID, commitmentInput.Slot))
		}

		bicInputSet[inp.AccountID] = accountData.Credits.Value
	}

	rewardInputSet := make(iotagovm.RewardsInputSet)
	for _, inp := range rewardInputs {
		output, ok := resolvedInputStates[inp.Index].(*utxoledger.Output)
		if !ok {
			return nil, ierrors.Wrapf(iotago.ErrRewardInputInvalid, "input at index %d is not an UTXO output", inp.Index)
		}
		outputID := output.OutputID()

		switch castOutput := output.Output().(type) {
		case *iotago.AccountOutput:
			stakingFeature := castOutput.FeatureSet().Staking()
			if stakingFeature == nil {
				return nil, ierrors.Wrapf(iotago.ErrNoStakingFeature, "cannot claim rewards from an AccountOutput %s at index %d without staking feature", outputID, inp.Index)
			}
			accountID := castOutput.AccountID
			if accountID.Empty() {
				accountID = iotago.AccountIDFromOutputID(outputID)
			}

			reward, _, _, rewardErr := v.ledger.sybilProtection.ValidatorReward(accountID, stakingFeature.StakedAmount, stakingFeature.StartEpoch, stakingFeature.EndEpoch)
			if rewardErr != nil {
				return nil, ierrors.Wrapf(iotago.ErrFailedToClaimStakingReward, "failed to get Validator reward for AccountOutput %s at index %d (StakedAmount: %d, StartEpoch: %d, EndEpoch: %d", outputID, inp.Index, stakingFeature.StakedAmount, stakingFeature.StartEpoch, stakingFeature.EndEpoch)
			}

			rewardInputSet[accountID] = reward

		case *iotago.DelegationOutput:
			delegationID := castOutput.DelegationID
			delegationEnd := castOutput.EndEpoch

			if delegationID.Empty() {
				delegationID = iotago.DelegationIDFromOutputID(outputID)

				// If Delegation ID is zeroed, the output is in delegating state, which means its End Epoch is not set and we must use the
				// "last epoch", which is the epoch index corresponding to the future bounded slot index minus 1.
				apiForSlot := v.ledger.apiProvider.APIForSlot(commitmentInput.Slot)
				futureBoundedSlotIndex := commitmentInput.Slot + apiForSlot.ProtocolParameters().MinCommittableAge()
				delegationEnd = apiForSlot.TimeProvider().EpochFromSlot(futureBoundedSlotIndex) - iotago.EpochIndex(1)
			}

			reward, _, _, rewardErr := v.ledger.sybilProtection.DelegatorReward(castOutput.ValidatorAddress.AccountID(), castOutput.DelegatedAmount, castOutput.StartEpoch, delegationEnd)
			if rewardErr != nil {
				return nil, ierrors.Wrapf(iotago.ErrFailedToClaimDelegationReward, "failed to get Delegator reward for DelegationOutput %s at index %d (StakedAmount: %d, StartEpoch: %d, EndEpoch: %d", outputID, inp.Index, castOutput.DelegatedAmount, castOutput.StartEpoch, castOutput.EndEpoch)
			}

			rewardInputSet[delegationID] = reward
		}
	}

	resolvedInputs := iotagovm.ResolvedInputs{
		InputSet:                    utxoInputSet,
		CommitmentInput:             commitmentInput,
		BlockIssuanceCreditInputSet: bicInputSet,
		RewardsInputSet:             rewardInputSet,
	}

	unlockedIdentities, err := nova.NewVirtualMachine().ValidateUnlocks(signedStardustTransaction, resolvedInputs)
	if err != nil {
		return nil, err
	}

	executionContext = context.Background()
	executionContext = context.WithValue(executionContext, ExecutionContextKeyUnlockedIdentities, unlockedIdentities)
	executionContext = context.WithValue(executionContext, ExecutionContextKeyResolvedInputs, resolvedInputs)

	return executionContext, nil
}

func (v *VM) Execute(executionContext context.Context, transaction mempool.Transaction) (outputs []mempool.State, err error) {
	stardustTransaction, ok := transaction.(*iotago.Transaction)
	if !ok {
		return nil, iotago.ErrTxTypeInvalid
	}

	transactionID, err := stardustTransaction.ID()
	if err != nil {
		return nil, err
	}

	unlockedIdentities, ok := executionContext.Value(ExecutionContextKeyUnlockedIdentities).(iotagovm.UnlockedIdentities)
	if !ok {
		return nil, ierrors.Errorf("unlockedIdentities not found in execution context")
	}

	resolvedInputs, ok := executionContext.Value(ExecutionContextKeyResolvedInputs).(iotagovm.ResolvedInputs)
	if !ok {
		return nil, ierrors.Errorf("resolvedInputs not found in execution context")
	}

	createdOutputs, err := nova.NewVirtualMachine().Execute(stardustTransaction, resolvedInputs, unlockedIdentities)
	if err != nil {
		return nil, err
	}

	for index, output := range createdOutputs {
		proof, err := iotago.OutputIDProofFromTransaction(stardustTransaction, uint16(index))
		if err != nil {
			return nil, err
		}

		outputs = append(outputs, utxoledger.CreateOutput(
			v.ledger.apiProvider,
			iotago.OutputIDFromTransactionIDAndIndex(transactionID, uint16(index)),
			iotago.EmptyBlockID,
			0,
			output,
			proof,
		))
	}

	return outputs, nil
}

// ExecutionContextKey is the type of the keys used in the execution context.
type ExecutionContextKey uint8

const (
	// ExecutionContextKeyUnlockedIdentities is the key for the unlocked identities in the execution context.
	ExecutionContextKeyUnlockedIdentities ExecutionContextKey = iota

	// ExecutionContextKeyResolvedInputs is the key for the resolved inputs in the execution context.
	ExecutionContextKeyResolvedInputs
)
