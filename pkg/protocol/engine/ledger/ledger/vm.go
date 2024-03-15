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
	iotagoTransaction, ok := transaction.(*iotago.Transaction)
	if !ok {
		return nil, iotago.ErrTxTypeInvalid
	}

	for _, input := range iotagoTransaction.TransactionEssence.Inputs {
		switch castedInput := input.(type) {
		case *iotago.UTXOInput:
			inputReferences = append(inputReferences, mempool.UTXOInputStateRefFromInput(
				castedInput,
			))
		default:
			// We're switching on the Go input type here, so we can only run into the default case
			// if we added a new input type and have not handled it above. In this case we want to panic.
			panic("all supported input types should be handled above")
		}
	}

	for _, contextInput := range iotagoTransaction.TransactionEssence.ContextInputs {
		switch castedContextInput := contextInput.(type) {
		case *iotago.CommitmentInput:
			inputReferences = append(inputReferences, mempool.CommitmentInputStateRefFromInput(
				castedContextInput,
			))
		// These context inputs do not need to be resolved.
		case *iotago.BlockIssuanceCreditInput, *iotago.RewardInput:
			continue
		default:
			// We're switching on the Go context input type here, so we can only run into the default case
			// if we added a new context input type and have not handled it above. In this case we want to panic.
			panic("all supported context input types should be handled above")
		}
	}

	return inputReferences, nil
}

func (v *VM) ValidateSignatures(signedTransaction mempool.SignedTransaction, resolvedInputStates []mempool.State) (executionContext context.Context, err error) {
	iotagoSignedTransaction, ok := signedTransaction.(*iotago.SignedTransaction)
	if !ok {
		return nil, iotago.ErrTxTypeInvalid
	}

	contextInputs := iotagoSignedTransaction.Transaction.ContextInputs()
	utxoInputSet := iotagovm.InputSet{}
	commitmentInput := (*iotago.Commitment)(nil)
	bicInputs := make([]*iotago.BlockIssuanceCreditInput, 0)
	rewardInputs := make([]*iotago.RewardInput, 0)

	for _, resolvedInput := range resolvedInputStates {
		switch typedInput := resolvedInput.(type) {
		case mempool.CommitmentInputState:
			commitmentInput = typedInput.Commitment
		case *utxoledger.Output:
			utxoInputSet[typedInput.OutputID()] = typedInput.Output()
		}
	}

	for _, contextInput := range contextInputs {
		switch typedInput := contextInput.(type) {
		case *iotago.BlockIssuanceCreditInput:
			bicInputs = append(bicInputs, typedInput)
		case *iotago.RewardInput:
			rewardInputs = append(rewardInputs, typedInput)
		}
	}

	if (len(rewardInputs) > 0 || len(bicInputs) > 0) && commitmentInput == nil {
		return nil, iotago.ErrCommitmentInputMissing
	}

	bicInputSet := make(iotagovm.BlockIssuanceCreditInputSet)
	for _, inp := range bicInputs {
		accountData, exists, accountErr := v.ledger.accountsLedger.Account(inp.AccountID, commitmentInput.Slot)
		if accountErr != nil {
			return nil, ierrors.Join(iotago.ErrBICInputReferenceInvalid, ierrors.Wrapf(accountErr, "could not get BIC input for account %s in slot %d", inp.AccountID, commitmentInput.Slot))
		}
		if !exists {
			return nil, ierrors.WithMessagef(iotago.ErrBICInputReferenceInvalid, "BIC input does not exist for account %s in slot %d", inp.AccountID, commitmentInput.Slot)
		}

		bicInputSet[inp.AccountID] = accountData.Credits.Value
	}

	rewardInputSet := make(iotagovm.RewardsInputSet)
	for _, inp := range rewardInputs {
		output, ok := resolvedInputStates[inp.Index].(*utxoledger.Output)
		if !ok {
			return nil, ierrors.WithMessagef(iotago.ErrRewardInputReferenceInvalid, "input at index %d is not a UTXO output", inp.Index)
		}
		outputID := output.OutputID()

		switch castOutput := output.Output().(type) {
		case *iotago.AccountOutput:
			stakingFeature := castOutput.FeatureSet().Staking()
			if stakingFeature == nil {
				return nil, ierrors.Wrapf(iotago.ErrRewardInputReferenceInvalid, "cannot claim rewards from a Account with output id %s at index %d without a staking feature", outputID.ToHex(), inp.Index)
			}
			accountID := castOutput.AccountID
			if accountID.Empty() {
				accountID = iotago.AccountIDFromOutputID(outputID)
			}

			apiForSlot := v.ledger.apiProvider.APIForSlot(commitmentInput.Slot)
			futureBoundedSlotIndex := commitmentInput.Slot + apiForSlot.ProtocolParameters().MinCommittableAge()
			claimingEpoch := apiForSlot.TimeProvider().EpochFromSlot(futureBoundedSlotIndex)

			reward, _, _, rewardErr := v.ledger.sybilProtection.ValidatorReward(accountID, stakingFeature, claimingEpoch)
			if rewardErr != nil {
				return nil, ierrors.Join(iotago.ErrStakingRewardCalculationFailure, ierrors.Wrapf(rewardErr, "failed to get validator reward for account with output id %s at index %d (StakedAmount: %d, StartEpoch: %d, EndEpoch: %d, claimingEpoch: %d)", outputID.ToHex(), inp.Index, stakingFeature.StakedAmount, stakingFeature.StartEpoch, stakingFeature.EndEpoch, claimingEpoch))
			}

			rewardInputSet[accountID] = reward

		case *iotago.DelegationOutput:
			delegationID := castOutput.DelegationID
			delegationEnd := castOutput.EndEpoch

			apiForSlot := v.ledger.apiProvider.APIForSlot(commitmentInput.Slot)
			futureBoundedSlotIndex := commitmentInput.Slot + apiForSlot.ProtocolParameters().MinCommittableAge()
			claimingEpoch := apiForSlot.TimeProvider().EpochFromSlot(futureBoundedSlotIndex)

			if delegationID.Empty() {
				delegationID = iotago.DelegationIDFromOutputID(outputID)

				// If Delegation ID is zeroed, the output is in delegating state, which means its End Epoch is not set and we must use the
				// "last epoch", which is the epoch index corresponding to the future bounded slot index minus 1.
				delegationEnd = claimingEpoch - iotago.EpochIndex(1)
			}

			reward, _, _, rewardErr := v.ledger.sybilProtection.DelegatorReward(castOutput.ValidatorAddress.AccountID(), castOutput.DelegatedAmount, castOutput.StartEpoch, delegationEnd, claimingEpoch)
			if rewardErr != nil {
				return nil, ierrors.Join(iotago.ErrDelegationRewardCalculationFailure, ierrors.Wrapf(rewardErr, "failed to get delegator reward for DelegationOutput with output id %s at index %d (DelegatedAmount: %d, StartEpoch: %d, EndEpoch: %d)", outputID, inp.Index, castOutput.DelegatedAmount, castOutput.StartEpoch, castOutput.EndEpoch))
			}

			rewardInputSet[delegationID] = reward
		default:
			return nil, ierrors.WithMessagef(iotago.ErrRewardInputReferenceInvalid, "reward input cannot point to %s", output.Output().Type())
		}
	}

	resolvedInputs := iotagovm.ResolvedInputs{
		InputSet:                    utxoInputSet,
		CommitmentInput:             commitmentInput,
		BlockIssuanceCreditInputSet: bicInputSet,
		RewardsInputSet:             rewardInputSet,
	}

	unlockedAddresses, err := nova.NewVirtualMachine().ValidateUnlocks(iotagoSignedTransaction, resolvedInputs)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to validate unlocks in signed transaction")
	}

	executionContext = context.Background()
	executionContext = context.WithValue(executionContext, ExecutionContextKeyUnlockedAddresses, unlockedAddresses)
	executionContext = context.WithValue(executionContext, ExecutionContextKeyResolvedInputs, resolvedInputs)

	return executionContext, nil
}

func (v *VM) Execute(executionContext context.Context, transaction mempool.Transaction) (outputs []mempool.State, err error) {
	iotagoTransaction, ok := transaction.(*iotago.Transaction)
	if !ok {
		return nil, iotago.ErrTxTypeInvalid
	}

	transactionID := iotagoTransaction.MustID()

	unlockedAddresses, ok := executionContext.Value(ExecutionContextKeyUnlockedAddresses).(iotagovm.UnlockedAddresses)
	if !ok {
		panic("unlockedAddresses should be present in execution context")
	}

	resolvedInputs, ok := executionContext.Value(ExecutionContextKeyResolvedInputs).(iotagovm.ResolvedInputs)
	if !ok {
		panic("resolvedInputs should be present in execution context")
	}

	createdOutputs, err := nova.NewVirtualMachine().Execute(iotagoTransaction, resolvedInputs, unlockedAddresses)
	if err != nil {
		return nil, err
	}

	for index, output := range createdOutputs {
		proof, err := iotago.OutputIDProofFromTransaction(iotagoTransaction, uint16(index))
		if err != nil {
			return nil, err
		}

		output := utxoledger.CreateOutput(
			v.ledger.apiProvider,
			iotago.OutputIDFromTransactionIDAndIndex(transactionID, uint16(index)),
			iotago.EmptyBlockID,
			0,
			output,
			proof,
		)

		outputs = append(outputs, output)
	}

	return outputs, nil
}

// ExecutionContextKey is the type of the keys used in the execution context.
type ExecutionContextKey uint8

const (
	// ExecutionContextKeyUnlockedAddresses is the key for the unlocked addresses in the execution context.
	ExecutionContextKeyUnlockedAddresses ExecutionContextKey = iota

	// ExecutionContextKeyResolvedInputs is the key for the resolved inputs in the execution context.
	ExecutionContextKeyResolvedInputs
)
