package retainer

import (
	"github.com/iotaledger/hive.go/ierrors"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/nodeclient/apimodels"
)

var txErrorsFailureReasonMap = map[error]apimodels.TransactionFailureReason{
	// unknown type / type casting errors
	iotago.ErrUnknownTransactinType:      apimodels.TxFailureUnderlyingTxTypeInvalid,
	iotago.ErrUnknownInputType:           apimodels.TxFailureUnderlyingUTXOInputInvalid,
	iotago.ErrFailedToRetrieveInput:      apimodels.TxFailureUnderlyingUTXOInputInvalid,
	iotago.ErrUnknownOutputType:          apimodels.TxFailureUnderlyingUTXOInputInvalid,
	iotago.ErrCouldNotResolveBICInput:    apimodels.TxFailureUnderlyingBICInputInvalid,
	iotago.ErrCouldNotResolveRewardInput: apimodels.TxFailureUnderlyingRewardInputInvalid,
	iotago.ErrCommitmentInputMissing:     apimodels.TxFailureUnderlyingCommitmentInputInvalid,
	iotago.ErrCouldNorRetrieveCommitment: apimodels.TxFailureUnderlyingCommitmentInputInvalid,

	// context inputs errors
	iotago.ErrNoStakingFeature:             apimodels.TxFailureNoStakingFeature,
	iotago.ErrFailedToClaimValidatorReward: apimodels.TxFailureFailedToClaimValidatorReward,
	iotago.ErrFailedToClaimDelegatorReward: apimodels.TxFailureFailedToClaimDelegatorReward,

	// native token errors
	iotago.ErrInvalidNativeTokenSet:        apimodels.TxFailureGivenNativeTokensInvalid,
	iotago.ErrMaxNativeTokensCountExceeded: apimodels.TxFailureGivenNativeTokensInvalid,
	iotago.ErrNativeTokenSumUnbalanced:     apimodels.TxFailureGivenNativeTokensInvalid,

	// vm errors
	iotago.ErrInputOutputSumMismatch:       apimodels.TxFailureSumOfInputAndOutputValuesDoesNotMatch,
	iotago.ErrTimelockNotExpired:           apimodels.TxFailureConfiguredTimelockNotYetExpired,
	iotago.ErrReturnAmountNotFulFilled:     apimodels.TxFailureReturnAmountNotFulfilled,
	iotago.ErrInvalidInputUnlock:           apimodels.TxFailureInputUnlockInvalid,
	iotago.ErrInvalidInputsCommitment:      apimodels.TxFailureInputsCommitmentInvalid,
	iotago.ErrSenderFeatureNotUnlocked:     apimodels.TxFailureSenderNotUnlocked,
	iotago.ErrChainTransitionInvalid:       apimodels.TxFailureChainStateTransitionInvalid,
	iotago.ErrInputOutputManaMismatch:      apimodels.TxFailureManaAmountInvalid,
	iotago.ErrInvalidManaAmount:            apimodels.TxFailureManaAmountInvalid,
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
