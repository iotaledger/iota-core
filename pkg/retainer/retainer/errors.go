package retainer

import (
	"github.com/iotaledger/hive.go/ierrors"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/nodeclient/apimodels"
)

var txErrorsFailureReasonMap = map[error]apimodels.TransactionFailureReason{
	iotago.ErrInputOutputSumMismatch:       apimodels.ErrTxStateSumOfInputAndOutputValuesDoesNotMatch,
	iotago.ErrTimelockNotExpired:           apimodels.ErrTxStateConfiguredTimelockNotYetExpired,
	iotago.ErrInvalidNativeTokenSet:        apimodels.ErrTxStateGivenNativeTokensInvalid,
	iotago.ErrMaxNativeTokensCountExceeded: apimodels.ErrTxStateGivenNativeTokensInvalid,
	iotago.ErrNativeTokenSumUnbalanced:     apimodels.ErrTxStateGivenNativeTokensInvalid,
	iotago.ErrReturnAmountNotFulFilled:     apimodels.ErrTxStateReturnAmountNotFulfilled,
	iotago.ErrFailedToUnlockInput:          apimodels.ErrTxStateInputUnlockInvalid,
	iotago.ErrInvalidInputUnlock:           apimodels.ErrTxStateInputUnlockInvalid,
	iotago.ErrInvalidInputsCommitment:      apimodels.ErrTxStateInputsCommitmentInvalid,
	iotago.ErrSenderFeatureNotUnlocked:     apimodels.ErrTxStateSenderNotUnlocked,
	iotago.ErrChainTransitionInvalid:       apimodels.ErrTxStateChainStateTransitionInvalid,
	iotago.ErrInputOutputManaMismatch:      apimodels.ErrTxStateManaAmount,
	iotago.ErrInvalidManaAmount:            apimodels.ErrTxStateManaAmount,
	iotago.ErrInputCreationAfterTxCreation: apimodels.ErrTxStateInputCreationAfterTxCreation,
	iotago.ErrUnknownOutputType:            apimodels.ErrTxStateUnderlyingUTXOInputInvalid,
	iotago.ErrUnknownTransactinType:        apimodels.ErrTxUnderlyingTxTypeInvalid,
	iotago.ErrFailedToRetrieveInput:        apimodels.ErrTxStateUnderlyingUTXOInputInvalid,
	iotago.ErrCouldNotResolveBICInput:      apimodels.ErrTxFailedToResolveBICInput,
	iotago.ErrCouldNotResolveRewardInput:   apimodels.ErrTxFailedToResolveRewardInput,
	iotago.ErrCommittmentInputMissing:      apimodels.ErrTxFailedToResolveCommitment,
	iotago.ErrNoStakingFeature:             apimodels.ErrTxStateNoStakingFeature,
	iotago.ErrFailedToClaimValidatorReward: apimodels.ErrTxStateFailedToClaimValidatorReward,
	iotago.ErrFailedToClaimDelegatorReward: apimodels.ErrTxStateFailedToClaimDelegatorReward,
}

func determineTxFailureReason(err error) apimodels.TransactionFailureReason {
	for errKey, failureReason := range txErrorsFailureReasonMap {
		if ierrors.Is(err, errKey) {
			return failureReason
		}
	}
	// use most general failure reason
	return apimodels.ErrTxStateSemanticValidationFailed
}
