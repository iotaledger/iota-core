package core

import (
	"fmt"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/iotaledger/hive.go/core/safemath"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/inx-app/pkg/httpserver"
	"github.com/iotaledger/iota-core/components/restapi"
	"github.com/iotaledger/iota-core/pkg/core/account"
	restapipkg "github.com/iotaledger/iota-core/pkg/restapi"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/nodeclient/apimodels"
)

func congestionForAccountID(c echo.Context) (*apimodels.CongestionResponse, error) {
	accountID, err := httpserver.ParseAccountIDParam(c, restapipkg.ParameterAccountID)
	if err != nil {
		return nil, err
	}

	slotIndex := deps.Protocol.MainEngineInstance().SyncManager.LatestCommitment().Index()

	acc, exists, err := deps.Protocol.MainEngineInstance().Ledger.Account(accountID, slotIndex)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to get account: %s form the Ledger", accountID.ToHex())
	}
	if !exists {
		return nil, ierrors.Errorf("account not found: %s", accountID.ToHex())
	}
	rmcSlot, err := safemath.SafeSub(slotIndex, deps.Protocol.APIForSlot(slotIndex).ProtocolParameters().MaxCommittableAge())
	if err != nil {
		rmcSlot = 0
	}
	rmc, err := deps.Protocol.CandidateEngineInstance().Ledger.RMCManager().RMC(rmcSlot)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to get RMC for slot: %d", rmcSlot)
	}

	return &apimodels.CongestionResponse{
		SlotIndex:            slotIndex,
		Ready:                deps.Protocol.MainEngineInstance().Scheduler.IsBlockIssuerReady(accountID),
		ReferenceManaCost:    rmc,
		BlockIssuanceCredits: acc.Credits.Value,
	}, nil
}

func validators(c echo.Context) (*apimodels.ValidatorsResponse, error) {
	var err error
	pageSize := restapi.ParamsRestAPI.MaxPageSize
	if len(c.QueryParam(restapipkg.QueryParameterPageSize)) > 0 {
		pageSize, err = httpserver.ParseUint32QueryParam(c, restapipkg.QueryParameterPageSize)
		if err != nil {
			return nil, ierrors.Wrapf(err, "failed to parse the %s parameter", restapipkg.QueryParameterPageSize)
		}
		if pageSize > restapi.ParamsRestAPI.MaxPageSize {
			pageSize = restapi.ParamsRestAPI.MaxPageSize
		}
	}
	latestCommittedSlot := deps.Protocol.MainEngineInstance().SyncManager.LatestCommitment().Index()
	// no cursor provided will be the first request
	requestedSlotIndex := latestCommittedSlot
	var cursorIndex uint32
	if len(c.QueryParam(restapipkg.QueryParameterCursor)) != 0 {
		requestedSlotIndex, cursorIndex, err = httpserver.ParseCursorQueryParam(c, restapipkg.QueryParameterCursor)
		if err != nil {
			return nil, ierrors.Wrapf(err, "failed to parse the %s parameter", restapipkg.QueryParameterCursor)
		}
	}

	// do not respond to really old requests
	if requestedSlotIndex+iotago.SlotIndex(restapi.ParamsRestAPI.MaxRequestedSlotAge) < latestCommittedSlot {
		return nil, ierrors.Errorf("request is too old, request started at %d, latest committed slot index is %d", requestedSlotIndex, latestCommittedSlot)
	}

	nextEpoch := deps.Protocol.APIForSlot(latestCommittedSlot).TimeProvider().EpochFromSlot(latestCommittedSlot) + 1

	slotRange := uint32(requestedSlotIndex) / restapi.ParamsRestAPI.RequestsMemoryCacheGranularity
	registeredValidators, exists := deps.Protocol.MainEngineInstance().Retainer.RegisteredValidatorsCache(slotRange)
	if !exists {
		registeredValidators, err = deps.Protocol.MainEngineInstance().SybilProtection.OrderedRegisteredCandidateValidatorsList(nextEpoch)
		if err != nil {
			return nil, ierrors.Wrapf(err, "failed to get ordered registered validators list for epoch %d", nextEpoch)
		}
		deps.Protocol.MainEngineInstance().Retainer.RetainRegisteredValidatorsCache(slotRange, registeredValidators)
	}

	page := registeredValidators[cursorIndex:lo.Min(cursorIndex+pageSize, uint32(len(registeredValidators)))]
	resp := &apimodels.ValidatorsResponse{
		Validators: page,
		PageSize:   pageSize,
	}
	// this is the last page
	if int(cursorIndex+pageSize) > len(registeredValidators) {
		resp.Cursor = ""
	} else {
		resp.Cursor = fmt.Sprintf("%d,%d", slotRange, cursorIndex+pageSize)
	}

	return resp, nil
}

func validatorByAccountID(c echo.Context) (*apimodels.ValidatorResponse, error) {
	accountID, err := httpserver.ParseAccountIDParam(c, restapipkg.ParameterAccountID)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to parse the %s parameter", restapipkg.ParameterAccountID)
	}
	latestCommittedSlot := deps.Protocol.MainEngineInstance().SyncManager.LatestCommitment().Index()

	accountData, exists, err := deps.Protocol.MainEngineInstance().Ledger.Account(accountID, latestCommittedSlot)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to get account: %s form the Ledger", accountID.ToHex())
	}
	if !exists {
		return nil, ierrors.Errorf("account not found: %s for latest committedSlot %d", accountID.ToHex(), latestCommittedSlot)
	}
	nextEpoch := deps.Protocol.APIForSlot(latestCommittedSlot).TimeProvider().EpochFromSlot(latestCommittedSlot) + 1
	active := deps.Protocol.MainEngineInstance().SybilProtection.IsCandidateActive(accountID, nextEpoch)

	return &apimodels.ValidatorResponse{
		AccountID:                      accountID,
		PoolStake:                      accountData.ValidatorStake + accountData.DelegationStake,
		ValidatorStake:                 accountData.ValidatorStake,
		StakingEpochEnd:                accountData.StakeEndEpoch,
		FixedCost:                      accountData.FixedCost,
		Active:                         active,
		LatestSupportedProtocolVersion: accountData.LatestSupportedProtocolVersionAndHash.Version,
		LatestSupportedProtocolHash:    accountData.LatestSupportedProtocolVersionAndHash.Hash,
	}, nil
}

func rewardsByOutputID(c echo.Context) (*apimodels.ManaRewardsResponse, error) {
	outputID, err := httpserver.ParseOutputIDParam(c, restapipkg.ParameterOutputID)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to parse the %s parameter", restapipkg.ParameterOutputID)
	}

	utxoOutput, err := deps.Protocol.MainEngineInstance().Ledger.Output(outputID)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to get output %s from ledger", outputID)
	}

	var reward iotago.Mana
	var actualStart, actualEnd iotago.EpochIndex
	switch utxoOutput.OutputType() {
	case iotago.OutputAccount:
		//nolint:forcetypeassert
		accountOutput := utxoOutput.Output().(*iotago.AccountOutput)
		feature, exists := accountOutput.FeatureSet()[iotago.FeatureStaking]
		if !exists {
			return nil, ierrors.Errorf("account %s is not a validator", outputID)
		}

		//nolint:forcetypeassert
		stakingFeature := feature.(*iotago.StakingFeature)

		// check if the account is a validator
		reward, actualStart, actualEnd, err = deps.Protocol.MainEngineInstance().SybilProtection.ValidatorReward(
			accountOutput.AccountID,
			stakingFeature.StakedAmount,
			stakingFeature.StartEpoch,
			stakingFeature.EndEpoch,
		)

	case iotago.OutputDelegation:
		//nolint:forcetypeassert
		delegationOutput := utxoOutput.Output().(*iotago.DelegationOutput)
		latestCommittedSlot := deps.Protocol.MainEngineInstance().SyncManager.LatestCommitment().Index()
		stakingEnd := delegationOutput.EndEpoch
		// the output is in delayed calaiming state if endEpoch is set, otherwise we use latest possible epoch
		if delegationOutput.EndEpoch == 0 {
			stakingEnd = deps.Protocol.APIForSlot(latestCommittedSlot).TimeProvider().EpochFromSlot(deps.Protocol.MainEngineInstance().SyncManager.LatestCommitment().Index())
		}
		reward, actualStart, actualEnd, err = deps.Protocol.MainEngineInstance().SybilProtection.DelegatorReward(
			delegationOutput.ValidatorID,
			delegationOutput.DelegatedAmount,
			delegationOutput.StartEpoch,
			stakingEnd,
		)
	}
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to calculate reward for output %s", outputID)
	}

	return &apimodels.ManaRewardsResponse{
		EpochStart: actualStart,
		EpochEnd:   actualEnd,
		Rewards:    reward,
	}, nil
}

func selectedCommittee(c echo.Context) *apimodels.CommitteeResponse {
	timeProvider := deps.Protocol.CurrentAPI().TimeProvider()

	var slotIndex iotago.SlotIndex

	epochIndex, err := httpserver.ParseEpochQueryParam(c, restapipkg.ParameterEpochIndex)
	if err != nil {
		// by default we return current epoch
		slotIndex = timeProvider.SlotFromTime(time.Now())
		epochIndex = timeProvider.EpochFromSlot(slotIndex)
	} else {
		slotIndex = timeProvider.EpochEnd(epochIndex)
	}

	seatedAccounts := deps.Protocol.MainEngineInstance().SybilProtection.SeatManager().Committee(slotIndex)
	committee := make([]*apimodels.CommitteeMemberResponse, 0, seatedAccounts.Accounts().Size())
	seatedAccounts.Accounts().ForEach(func(accountID iotago.AccountID, seat *account.Pool) bool {
		committee = append(committee, &apimodels.CommitteeMemberResponse{
			AccountID:      accountID,
			PoolStake:      seat.PoolStake,
			ValidatorStake: seat.ValidatorStake,
			FixedCost:      seat.FixedCost,
		})

		return true
	})

	return &apimodels.CommitteeResponse{
		EpochIndex:          epochIndex,
		Committee:           committee,
		TotalStake:          seatedAccounts.Accounts().TotalStake(),
		TotalValidatorStake: seatedAccounts.Accounts().TotalValidatorStake(),
	}
}
