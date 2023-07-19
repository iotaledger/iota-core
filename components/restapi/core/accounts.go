package core

import (
	"time"

	"github.com/labstack/echo/v4"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/inx-app/pkg/httpserver"
	"github.com/iotaledger/iota-core/pkg/core/account"
	restapipkg "github.com/iotaledger/iota-core/pkg/restapi"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/nodeclient/models"
)

func blockIssuanceCreditsForAccountID(c echo.Context) (*models.BlockIssuanceCreditsResponse, error) {
	accountID, err := httpserver.ParseAccountIDParam(c, restapipkg.ParameterAccountID)
	if err != nil {
		return nil, err
	}
	slotIndex, err := httpserver.ParseSlotQueryParam(c, restapipkg.ParameterSlotIndex)
	if err != nil {
		// by default we return the balance for the latest slot
		slotIndex = deps.Protocol.SyncManager.LatestCommittedSlot()
	}
	account, exists, err := deps.Protocol.MainEngineInstance().Ledger.Account(accountID, slotIndex)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ierrors.Errorf("account not found: %s", accountID.ToHex())
	}

	return &models.BlockIssuanceCreditsResponse{
		SlotIndex:            slotIndex,
		BlockIssuanceCredits: account.Credits.Value,
	}, nil
}

func congestionForAccountID(c echo.Context) (*models.CongestionResponse, error) {
	accountID, err := httpserver.ParseAccountIDParam(c, restapipkg.ParameterAccountID)
	if err != nil {
		return nil, err
	}
	mca := deps.Protocol.LatestAPI().ProtocolParameters().EvictionAge()
	slotIndex := deps.Protocol.LatestAPI().TimeProvider().SlotFromTime(time.Now())
	if slotIndex < mca {
		mca = 0
	}
	account, exists, err := deps.Protocol.MainEngineInstance().Ledger.Account(accountID, slotIndex-mca)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ierrors.Errorf("account not found: %s", accountID.ToHex())
	}

	return &models.CongestionResponse{
		SlotIndex:            slotIndex,
		Ready:                false, // TODO: update after scheduler is implemented
		ReferenceManaCost:    0,     // TODO: update after RMC is implemented
		BlockIssuanceCredits: account.Credits.Value,
	}, nil
}

func staking() (*models.AccountStakingListResponse, error) {
	resp := &models.AccountStakingListResponse{
		Stakers: make([]models.ValidatorResponse, 0),
	}
	latestCommittedSlot := deps.Protocol.SyncManager.LatestCommittedSlot()
	nextEpoch := deps.Protocol.APIForSlot(latestCommittedSlot).TimeProvider().EpochFromSlot(latestCommittedSlot) + 1

	activeValidators, err := deps.Protocol.MainEngineInstance().SybilProtection.EligibleValidators(nextEpoch)
	if err != nil {
		return nil, err
	}

	for _, accountData := range activeValidators {
		resp.Stakers = append(resp.Stakers, models.ValidatorResponse{
			AccountID:                      accountData.ID.ToHex(),
			PoolStake:                      accountData.ValidatorStake + accountData.DelegationStake,
			ValidatorStake:                 accountData.ValidatorStake,
			FixedCost:                      accountData.FixedCost,
			StakingEpochEnd:                accountData.StakeEndEpoch,
			LatestSupportedProtocolVersion: 1, // TODO: update after protocol versioning is included in the account ledger
		})
	}

	return resp, nil
}

func stakingByAccountID(c echo.Context) (*models.ValidatorResponse, error) {
	accountID, err := httpserver.ParseAccountIDParam(c, restapipkg.ParameterAccountID)
	if err != nil {
		return nil, err
	}
	latestCommittedSlot := deps.Protocol.SyncManager.LatestCommittedSlot()

	accountData, exists, err := deps.Protocol.MainEngineInstance().Ledger.Account(accountID, latestCommittedSlot)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ierrors.Errorf("account not found: %s for latest committedSlot %d", accountID.ToHex(), latestCommittedSlot)
	}

	return &models.ValidatorResponse{
		AccountID:                      accountID.ToHex(),
		PoolStake:                      accountData.ValidatorStake + accountData.DelegationStake,
		ValidatorStake:                 accountData.ValidatorStake,
		StakingEpochEnd:                accountData.StakeEndEpoch,
		FixedCost:                      accountData.FixedCost,
		LatestSupportedProtocolVersion: 1, // TODO: update after protocol versioning is included in the account ledger
	}, nil
}

func rewardsByAccountID(c echo.Context) (*models.ManaRewardsResponse, error) {
	outputID, err := httpserver.ParseOutputIDParam(c, restapipkg.ParameterAccountID)
	if err != nil {
		return nil, err
	}
	latestCommittedSlot := deps.Protocol.SyncManager.LatestCommittedSlot()
	latestRewardsReadyEpoch := deps.Protocol.APIForSlot(latestCommittedSlot).TimeProvider().EpochFromSlot(latestCommittedSlot)

	utxoOutput, err := deps.Protocol.MainEngineInstance().Ledger.Output(outputID)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to get output %s from ledger", outputID)
	}
	var reward iotago.Mana
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
		reward, err = deps.Protocol.MainEngineInstance().SybilProtection.ValidatorReward(
			accountOutput.AccountID,
			stakingFeature.StakedAmount,
			stakingFeature.StartEpoch,
			stakingFeature.EndEpoch,
		)

	case iotago.OutputDelegation:
		//nolint:forcetypeassert
		delegationOutput := utxoOutput.Output().(*iotago.DelegationOutput)
		reward, err = deps.Protocol.MainEngineInstance().SybilProtection.DelegatorReward(
			delegationOutput.ValidatorID,
			delegationOutput.DelegatedAmount,
			delegationOutput.StartEpoch,
			delegationOutput.EndEpoch,
		)
	}
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to calculate reward for output %s", outputID)
	}

	return &models.ManaRewardsResponse{
		EpochIndex: latestRewardsReadyEpoch,
		Rewards:    reward,
	}, nil
}

func selectedCommittee(c echo.Context) *models.CommitteeResponse {
	timeProvider := deps.Protocol.APIForSlot(deps.Protocol.SyncManager.LatestCommittedSlot()).TimeProvider()

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
	committee := make([]models.CommitteeMemberResponse, 0, seatedAccounts.Accounts().Size())
	seatedAccounts.Accounts().ForEach(func(accountID iotago.AccountID, seat *account.Pool) bool {
		committee = append(committee, models.CommitteeMemberResponse{
			AccountID:      accountID.ToHex(),
			PoolStake:      seat.PoolStake,
			ValidatorStake: seat.ValidatorStake,
			FixedCost:      seat.FixedCost,
		})

		return true
	})

	return &models.CommitteeResponse{
		EpochIndex:          epochIndex,
		Committee:           committee,
		TotalStake:          seatedAccounts.Accounts().TotalStake(),
		TotalValidatorStake: seatedAccounts.Accounts().TotalValidatorStake(),
	}
}
