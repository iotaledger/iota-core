package retainer

import (
	"github.com/iotaledger/hive.go/ierrors"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/nodeclient/apimodels"
)

var txErrorsFailureReasonMap = map[error]apimodels.TransactionFailureReason{
	// unknown type / type casting errors
	iotago.ErrUnknownTransactinType:      apimodels.ErrTxStateUnderlyingTxTypeInvalid,
	iotago.ErrFailedToRetrieveInput:      apimodels.ErrTxStateUnderlyingUTXOInputInvalid,
	iotago.ErrUnknownOutputType:          apimodels.ErrTxStateUnderlyingUTXOInputInvalid,
	iotago.ErrCouldNotResolveBICInput:    apimodels.ErrTxStateUnderlyingBICInputInvalid,
	iotago.ErrCouldNotResolveRewardInput: apimodels.ErrTxStateUnderlyingRewardInputInvalid,
	iotago.ErrCommittmentInputMissing:    apimodels.ErrTxStateUnderlyingCommitmentInputInvalid,
	iotago.ErrCouldNorRetrieveCommitment: apimodels.ErrTxStateUnderlyingCommitmentInputInvalid,

	// context inputs errors
	iotago.ErrNoStakingFeature:             apimodels.ErrTxStateNoStakingFeature,
	iotago.ErrFailedToClaimValidatorReward: apimodels.ErrTxStateFailedToClaimValidatorReward,
	iotago.ErrFailedToClaimDelegatorReward: apimodels.ErrTxStateFailedToClaimDelegatorReward,

	// native token errors
	iotago.ErrInvalidNativeTokenSet:        apimodels.ErrTxStateGivenNativeTokensInvalid,
	iotago.ErrMaxNativeTokensCountExceeded: apimodels.ErrTxStateGivenNativeTokensInvalid,
	iotago.ErrNativeTokenSumUnbalanced:     apimodels.ErrTxStateGivenNativeTokensInvalid,

	// vm errors
	iotago.ErrInputOutputSumMismatch:       apimodels.ErrTxStateSumOfInputAndOutputValuesDoesNotMatch,
	iotago.ErrTimelockNotExpired:           apimodels.ErrTxStateConfiguredTimelockNotYetExpired,
	iotago.ErrReturnAmountNotFulFilled:     apimodels.ErrTxStateReturnAmountNotFulfilled,
	iotago.ErrInvalidInputUnlock:           apimodels.ErrTxStateInputUnlockInvalid,
	iotago.ErrInvalidInputsCommitment:      apimodels.ErrTxStateInputsCommitmentInvalid,
	iotago.ErrSenderFeatureNotUnlocked:     apimodels.ErrTxStateSenderNotUnlocked,
	iotago.ErrChainTransitionInvalid:       apimodels.ErrTxStateChainStateTransitionInvalid,
	iotago.ErrInputOutputManaMismatch:      apimodels.ErrTxStateManaAmount,
	iotago.ErrInvalidManaAmount:            apimodels.ErrTxStateManaAmount,
	iotago.ErrInputCreationAfterTxCreation: apimodels.ErrTxStateInputCreationAfterTxCreation,
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
