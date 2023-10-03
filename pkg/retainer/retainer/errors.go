package retainer

import (
	"github.com/iotaledger/hive.go/ierrors"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/nodeclient/apimodels"
)

var txErrorsFailureReasonMap = map[error]apimodels.TransactionFailureReason{
	// unknown type / type casting errors
	iotago.ErrTxTypeInvalid:               apimodels.TxFailureTxTypeInvalid,
	iotago.ErrUnknownInputType:            apimodels.TxFailureUTXOInputInvalid,
	iotago.ErrUTXOInputInvalid:            apimodels.TxFailureUTXOInputInvalid,
	iotago.ErrUnknownOutputType:           apimodels.TxFailureUTXOInputInvalid,
	iotago.ErrBICInputInvalid:             apimodels.TxFailureBICInputInvalid,
	iotago.ErrRewardInputInvalid:          apimodels.TxFailureRewardInputInvalid,
	iotago.ErrCommitmentInputMissing:      apimodels.TxFailureCommitmentInputInvalid,
	iotago.ErrCommitmentInputInvalid:      apimodels.TxFailureCommitmentInputInvalid,
	iotago.ErrUnlockBlockSignatureInvalid: apimodels.TxFailureUnlockBlockSignatureInvalid,

	// context inputs errors
	iotago.ErrNoStakingFeature:              apimodels.TxFailureNoStakingFeature,
	iotago.ErrFailedToClaimStakingReward:    apimodels.TxFailureFailedToClaimStakingReward,
	iotago.ErrFailedToClaimDelegationReward: apimodels.TxFailureFailedToClaimDelegationReward,

	// UTXO errors
	iotago.ErrTxConflicting:     apimodels.TxFailureConflicting,
	iotago.ErrInputAlreadySpent: apimodels.TxFailureUTXOInputAlreadySpent,

	// native token errors
	iotago.ErrNativeTokenSetInvalid:    apimodels.TxFailureGivenNativeTokensInvalid,
	iotago.ErrNativeTokenSumUnbalanced: apimodels.TxFailureGivenNativeTokensInvalid,

	// vm errors
	iotago.ErrInputOutputSumMismatch:       apimodels.TxFailureSumOfInputAndOutputValuesDoesNotMatch,
	iotago.ErrTimelockNotExpired:           apimodels.TxFailureConfiguredTimelockNotYetExpired,
	iotago.ErrReturnAmountNotFulFilled:     apimodels.TxFailureReturnAmountNotFulfilled,
	iotago.ErrInvalidInputUnlock:           apimodels.TxFailureInputUnlockInvalid,
	iotago.ErrInvalidInputsCommitment:      apimodels.TxFailureInputsCommitmentInvalid,
	iotago.ErrSenderFeatureNotUnlocked:     apimodels.TxFailureSenderNotUnlocked,
	iotago.ErrChainTransitionInvalid:       apimodels.TxFailureChainStateTransitionInvalid,
	iotago.ErrInputOutputManaMismatch:      apimodels.TxFailureManaAmountInvalid,
	iotago.ErrManaAmountInvalid:            apimodels.TxFailureManaAmountInvalid,
	iotago.ErrInputCreationAfterTxCreation: apimodels.TxFailureInputCreationAfterTxCreation,
}

func determineTxFailureReason(err error) apimodels.TransactionFailureReason {
	for errKey, failureReason := range txErrorsFailureReasonMap {
		if ierrors.Is(err, errKey) {
			return failureReason
		}
	}
	// use most general failure reason
	return apimodels.TxFailureSemanticValidationFailed
}
