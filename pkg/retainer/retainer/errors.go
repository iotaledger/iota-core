package retainer

import (
	"github.com/iotaledger/hive.go/ierrors"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

var blocksErrorsFailureReasonMap = map[error]api.BlockFailureReason{
	iotago.ErrIssuerAccountNotFound:     api.BlockFailureIssuerAccountNotFound,
	iotago.ErrBurnedInsufficientMana:    api.BlockFailureBurnedInsufficientMana,
	iotago.ErrBlockVersionInvalid:       api.BlockFailureVersionInvalid,
	iotago.ErrRMCNotFound:               api.BlockFailureAccountInvalid,
	iotago.ErrFailedToCalculateManaCost: api.BlockFailureManaCostCalculationFailed,
	iotago.ErrNegativeBIC:               api.BlockFailureAccountInvalid,
	iotago.ErrAccountExpired:            api.BlockFailureAccountInvalid,
	iotago.ErrInvalidSignature:          api.BlockFailureSignatureInvalid,
}

func determineBlockFailureReason(err error) api.BlockFailureReason {
	for errKey, failureReason := range blocksErrorsFailureReasonMap {
		if ierrors.Is(err, errKey) {
			return failureReason
		}
	}
	// use most general failure reason
	return api.BlockFailureInvalid
}

var txErrorsFailureReasonMap = map[error]api.TransactionFailureReason{
	// unknown type / type casting errors
	iotago.ErrTxTypeInvalid:               api.TxFailureTxTypeInvalid,
	iotago.ErrUnknownInputType:            api.TxFailureUTXOInputInvalid,
	iotago.ErrUTXOInputInvalid:            api.TxFailureUTXOInputInvalid,
	iotago.ErrUnknownOutputType:           api.TxFailureUTXOInputInvalid,
	iotago.ErrBICInputInvalid:             api.TxFailureBICInputInvalid,
	iotago.ErrRewardInputInvalid:          api.TxFailureRewardInputInvalid,
	iotago.ErrCommitmentInputMissing:      api.TxFailureCommitmentInputInvalid,
	iotago.ErrCommitmentInputInvalid:      api.TxFailureCommitmentInputInvalid,
	iotago.ErrUnlockBlockSignatureInvalid: api.TxFailureUnlockBlockSignatureInvalid,

	// context inputs errors
	iotago.ErrNoStakingFeature:              api.TxFailureNoStakingFeature,
	iotago.ErrFailedToClaimStakingReward:    api.TxFailureFailedToClaimStakingReward,
	iotago.ErrFailedToClaimDelegationReward: api.TxFailureFailedToClaimDelegationReward,

	// UTXO errors
	iotago.ErrTxConflicting:     api.TxFailureConflicting,
	iotago.ErrInputAlreadySpent: api.TxFailureUTXOInputAlreadySpent,

	// native token errors
	iotago.ErrNativeTokenSetInvalid:    api.TxFailureGivenNativeTokensInvalid,
	iotago.ErrNativeTokenSumUnbalanced: api.TxFailureGivenNativeTokensInvalid,

	// vm errors
	iotago.ErrInputOutputSumMismatch:       api.TxFailureSumOfInputAndOutputValuesDoesNotMatch,
	iotago.ErrTimelockNotExpired:           api.TxFailureConfiguredTimelockNotYetExpired,
	iotago.ErrReturnAmountNotFulFilled:     api.TxFailureReturnAmountNotFulfilled,
	iotago.ErrInvalidInputUnlock:           api.TxFailureInputUnlockInvalid,
	iotago.ErrSenderFeatureNotUnlocked:     api.TxFailureSenderNotUnlocked,
	iotago.ErrChainTransitionInvalid:       api.TxFailureChainStateTransitionInvalid,
	iotago.ErrInputOutputManaMismatch:      api.TxFailureManaAmountInvalid,
	iotago.ErrManaAmountInvalid:            api.TxFailureManaAmountInvalid,
	iotago.ErrInputCreationAfterTxCreation: api.TxFailureInputCreationAfterTxCreation,

	// tx capabilities errors
	iotago.ErrTxCapabilitiesNativeTokenBurningNotAllowed: api.TxFailureCapabilitiesNativeTokenBurningNotAllowed,
	iotago.ErrTxCapabilitiesManaBurningNotAllowed:        api.TxFailureCapabilitiesManaBurningNotAllowed,
	iotago.ErrTxCapabilitiesAccountDestructionNotAllowed: api.TxFailureCapabilitiesAccountDestructionNotAllowed,
	iotago.ErrTxCapabilitiesAnchorDestructionNotAllowed:  api.TxFailureCapabilitiesAnchorDestructionNotAllowed,
	iotago.ErrTxCapabilitiesFoundryDestructionNotAllowed: api.TxFailureCapabilitiesFoundryDestructionNotAllowed,
	iotago.ErrTxCapabilitiesNFTDestructionNotAllowed:     api.TxFailureCapabilitiesNFTDestructionNotAllowed,
}

func determineTxFailureReason(err error) api.TransactionFailureReason {
	for errKey, failureReason := range txErrorsFailureReasonMap {
		if ierrors.Is(err, errKey) {
			return failureReason
		}
	}
	// use most general failure reason
	return api.TxFailureSemanticValidationFailed
}
