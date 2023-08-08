package core

import (
	"fmt"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/inx-app/pkg/httpserver"
	"github.com/iotaledger/iota-core/pkg/core/account"
	restapipkg "github.com/iotaledger/iota-core/pkg/restapi"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/nodeclient/apimodels"
)

const (
	RequestsMemoryCacheGranularity = 10
	MaxRequestedSlotAge            = 10
)

func congestionForAccountID(c echo.Context) (*apimodels.CongestionResponse, error) {
	accountID, err := httpserver.ParseAccountIDParam(c, restapipkg.ParameterAccountID)
	if err != nil {
		return nil, err
	}

	slotIndex := deps.Protocol.SyncManager.LatestCommitment().Index()

	acc, exists, err := deps.Protocol.MainEngineInstance().Ledger.Account(accountID, slotIndex)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ierrors.Errorf("account not found: %s", accountID.ToHex())
	}

	return &apimodels.CongestionResponse{
		SlotIndex:            slotIndex,
		Ready:                deps.Protocol.MainEngineInstance().Scheduler.IsBlockIssuerReady(accountID),
		ReferenceManaCost:    0, // TODO: update after RMC is implemented
		BlockIssuanceCredits: acc.Credits.Value,
	}, nil
}

func staking(c echo.Context) (*apimodels.AccountStakingListResponse, error) {
	pageSize, _ := httpserver.ParseUint32QueryParam(c, restapipkg.QueryParameterPageSize)
	requestedSlotIndex, cursorIndex, _ := httpserver.ParseCursorQueryParam(c, restapipkg.QueryParameterCursor)
	latestCommittedSlot := deps.Protocol.SyncManager.LatestCommitment().Index()

	if cursorIndex == 0 {
		requestedSlotIndex = latestCommittedSlot
	} else {
		// do not respond to really old requests
		if requestedSlotIndex+MaxRequestedSlotAge < latestCommittedSlot {
			return nil, ierrors.Errorf("request is too old, request started at %d, latest committed slot index is %d", requestedSlotIndex, latestCommittedSlot)
		}
	}

	nextEpoch := deps.Protocol.APIForSlot(latestCommittedSlot).TimeProvider().EpochFromSlot(latestCommittedSlot) + 1

	slotRange := uint32(requestedSlotIndex / RequestsMemoryCacheGranularity)
	registeredValidators, exists := deps.Protocol.MainEngineInstance().Retainer.RegisteredValidatorsCache(slotRange)
	if !exists {
		var err error
		registeredValidators, err = deps.Protocol.MainEngineInstance().SybilProtection.OrderedRegisteredValidatorsList(nextEpoch)
		if err != nil {
			return nil, err
		}
		deps.Protocol.MainEngineInstance().Retainer.RetainRegisteredValidatorsCache(slotRange, registeredValidators)
	}
	page := registeredValidators[cursorIndex : cursorIndex+pageSize]
	resp := &apimodels.AccountStakingListResponse{
		Stakers:  page,
		PageSize: pageSize,
		Cursor:   fmt.Sprintf("%d,%d", slotRange, cursorIndex+pageSize),
	}

	return resp, nil
}

func stakingByAccountID(c echo.Context) (*apimodels.ValidatorResponse, error) {
	accountID, err := httpserver.ParseAccountIDParam(c, restapipkg.ParameterAccountID)
	if err != nil {
		return nil, err
	}
	latestCommittedSlot := deps.Protocol.SyncManager.LatestCommitment().Index()

	accountData, exists, err := deps.Protocol.MainEngineInstance().Ledger.Account(accountID, latestCommittedSlot)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ierrors.Errorf("account not found: %s for latest committedSlot %d", accountID.ToHex(), latestCommittedSlot)
	}
	nextEpoch := deps.Protocol.APIForSlot(latestCommittedSlot).TimeProvider().EpochFromSlot(latestCommittedSlot) + 1
	active := deps.Protocol.MainEngineInstance().SybilProtection.IsActive(accountID, nextEpoch)

	return &apimodels.ValidatorResponse{
		AccountID:                      accountID,
		PoolStake:                      accountData.ValidatorStake + accountData.DelegationStake,
		ValidatorStake:                 accountData.ValidatorStake,
		StakingEpochEnd:                accountData.StakeEndEpoch,
		FixedCost:                      accountData.FixedCost,
		Active:                         active,
		LatestSupportedProtocolVersion: 1, // TODO: update after protocol versioning is included in the account ledger
	}, nil
}

func rewardsByOutputID(c echo.Context) (*apimodels.ManaRewardsResponse, error) {
	outputID, err := httpserver.ParseOutputIDParam(c, restapipkg.ParameterOutputID)
	if err != nil {
		return nil, err
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
		reward, actualStart, actualEnd, err = deps.Protocol.MainEngineInstance().SybilProtection.DelegatorReward(
			delegationOutput.ValidatorID,
			delegationOutput.DelegatedAmount,
			delegationOutput.StartEpoch,
			delegationOutput.EndEpoch,
		)
	}
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to calculate reward for output %s", outputID)
	}

	return &apimodels.ManaRewardsResponse{
		EpochIndexStart: actualStart,
		EpochIndexEnd:   actualEnd,
		Rewards:         reward,
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
