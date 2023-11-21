package core

import (
	"fmt"
	"time"

	"github.com/labstack/echo/v4"

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
	commitmentID, err := httpserver.ParseCommitmentIDQueryParam(c, restapipkg.ParameterCommitmentID)
	if err != nil {
		return nil, err
	}

	commitment := deps.Protocol.MainEngineInstance().SyncManager.LatestCommitment()
	if commitmentID != iotago.EmptyCommitmentID {
		// a commitment ID was provided, so we use the commitment for that ID
		commitment, err = getCommitmentByID(commitmentID, commitment)
		if err != nil {
			return nil, err
		}
	}

	hrp := deps.Protocol.CommittedAPI().ProtocolParameters().Bech32HRP()
	address, err := httpserver.ParseBech32AddressParam(c, hrp, restapipkg.ParameterBech32Address)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to parse bech32 address %s", c.Param(restapipkg.ParameterBech32Address))
	}

	accountAddress, ok := address.Clone().(*iotago.AccountAddress)
	if !ok {
		return nil, ierrors.Wrapf(echo.ErrInternalServerError, "failed to parse bech32 address %s", c.Param(restapipkg.ParameterBech32Address))
	}

	accountID := accountAddress.AccountID()
	acc, exists, err := deps.Protocol.MainEngineInstance().Ledger.Account(accountID, commitment.Slot())
	if err != nil {
		return nil, ierrors.Wrapf(echo.ErrInternalServerError, "failed to get account %s from the Ledger: %s", accountID.ToHex(), err)
	}
	if !exists {
		return nil, ierrors.Wrapf(echo.ErrNotFound, "account not found: %s", accountID.ToHex())
	}

	return &apimodels.CongestionResponse{
		Slot:                 commitment.Slot(),
		Ready:                deps.Protocol.MainEngineInstance().Scheduler.IsBlockIssuerReady(accountID),
		ReferenceManaCost:    commitment.ReferenceManaCost(),
		BlockIssuanceCredits: acc.Credits.Value,
	}, nil
}

func validators(c echo.Context) (*apimodels.ValidatorsResponse, error) {
	var err error
	pageSize := restapi.ParamsRestAPI.MaxPageSize
	if len(c.QueryParam(restapipkg.QueryParameterPageSize)) > 0 {
		pageSize, err = httpserver.ParseUint32QueryParam(c, restapipkg.QueryParameterPageSize)
		if err != nil {
			return nil, ierrors.Wrapf(err, "failed to parse page size %s", c.Param(restapipkg.QueryParameterPageSize))
		}
		if pageSize > restapi.ParamsRestAPI.MaxPageSize {
			pageSize = restapi.ParamsRestAPI.MaxPageSize
		}
	}
	latestCommittedSlot := deps.Protocol.MainEngineInstance().SyncManager.LatestCommitment().Slot()
	// no cursor provided will be the first request
	requestedSlot := latestCommittedSlot
	var cursorIndex uint32
	if len(c.QueryParam(restapipkg.QueryParameterCursor)) != 0 {
		requestedSlot, cursorIndex, err = httpserver.ParseCursorQueryParam(c, restapipkg.QueryParameterCursor)
		if err != nil {
			return nil, ierrors.Wrapf(err, "failed to parse cursor %s", c.Param(restapipkg.QueryParameterCursor))
		}
	}

	// do not respond to really old requests
	if requestedSlot+iotago.SlotIndex(restapi.ParamsRestAPI.MaxRequestedSlotAge) < latestCommittedSlot {
		return nil, ierrors.Wrapf(echo.ErrBadRequest, "request is too old, request started at %d, latest committed slot index is %d", requestedSlot, latestCommittedSlot)
	}

	nextEpoch := deps.Protocol.APIForSlot(latestCommittedSlot).TimeProvider().EpochFromSlot(latestCommittedSlot) + 1

	slotRange := uint32(requestedSlot) / restapi.ParamsRestAPI.RequestsMemoryCacheGranularity
	registeredValidators, exists := deps.Protocol.MainEngineInstance().Retainer.RegisteredValidatorsCache(slotRange)
	if !exists {
		registeredValidators, err = deps.Protocol.MainEngineInstance().SybilProtection.OrderedRegisteredCandidateValidatorsList(nextEpoch)
		if err != nil {
			return nil, ierrors.Wrapf(echo.ErrInternalServerError, "failed to get ordered registered validators list for epoch %d : %s", nextEpoch, err)
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
		return nil, ierrors.Wrapf(err, "failed to parse account ID %s", c.Param(restapipkg.ParameterAccountID))
	}
	latestCommittedSlot := deps.Protocol.MainEngineInstance().SyncManager.LatestCommitment().Slot()

	accountData, exists, err := deps.Protocol.MainEngineInstance().Ledger.Account(accountID, latestCommittedSlot)
	if err != nil {
		return nil, ierrors.Wrapf(echo.ErrInternalServerError, "failed to get account %s from the Ledger: %s", accountID.ToHex(), err)
	}
	if !exists {
		return nil, ierrors.Wrapf(echo.ErrNotFound, "account %s not found for latest committedSlot %d", accountID.ToHex(), latestCommittedSlot)
	}
	nextEpoch := deps.Protocol.APIForSlot(latestCommittedSlot).TimeProvider().EpochFromSlot(latestCommittedSlot) + 1

	active, err := deps.Protocol.MainEngineInstance().SybilProtection.IsCandidateActive(accountID, nextEpoch)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to check if account %s is an active candidate", accountID.ToHex())
	}

	return &apimodels.ValidatorResponse{
		AddressBech32:                  accountID.ToAddress().Bech32(deps.Protocol.CommittedAPI().ProtocolParameters().Bech32HRP()),
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
		return nil, ierrors.Wrapf(err, "failed to parse output ID %s", c.Param(restapipkg.ParameterOutputID))
	}

	var slotIndex iotago.SlotIndex
	if len(c.QueryParam(restapipkg.ParameterSlotIndex)) > 0 {
		var err error
		slotIndex, err = httpserver.ParseSlotQueryParam(c, restapipkg.ParameterSlotIndex)
		if err != nil {
			return nil, ierrors.Wrapf(err, "failed to parse slot index %s", c.Param(restapipkg.ParameterSlotIndex))
		}
		genesisSlot := deps.Protocol.LatestAPI().ProtocolParameters().GenesisSlot()
		if slotIndex < genesisSlot {
			return nil, ierrors.Wrapf(echo.ErrBadRequest, "slot index (%d) before genesis slot (%d)", slotIndex, genesisSlot)
		}
	} else {
		// The slot index may be unset for requests that do not want to issue a transaction, such as displaying estimated rewards,
		// in which case we use latest committed slot.
		slotIndex = deps.Protocol.MainEngineInstance().SyncManager.LatestCommitment().Slot()
	}

	utxoOutput, err := deps.Protocol.MainEngineInstance().Ledger.Output(outputID)
	if err != nil {
		return nil, ierrors.Wrapf(echo.ErrInternalServerError, "failed to get output %s from ledger: %s", outputID.ToHex(), err)
	}

	var reward iotago.Mana
	var actualStart, actualEnd iotago.EpochIndex
	switch utxoOutput.OutputType() {
	case iotago.OutputAccount:
		//nolint:forcetypeassert
		accountOutput := utxoOutput.Output().(*iotago.AccountOutput)
		feature, exists := accountOutput.FeatureSet()[iotago.FeatureStaking]
		if !exists {
			return nil, ierrors.Wrapf(echo.ErrBadRequest, "account %s is not a validator", outputID.ToHex())
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
		delegationEnd := delegationOutput.EndEpoch
		// If Delegation ID is zeroed, the output is in delegating state, which means its End Epoch is not set and we must use the
		// "last epoch" for the rewards calculation.
		// In this case the calculation must be consistent with the rewards calculation at execution time, so a client can specify
		// a slot index explicitly, which should be equal to the slot it uses as the commitment input for the claiming transaction.
		if delegationOutput.DelegationID.Empty() {
			apiForSlot := deps.Protocol.APIForSlot(slotIndex)
			futureBoundedSlotIndex := slotIndex + apiForSlot.ProtocolParameters().MinCommittableAge()
			delegationEnd = apiForSlot.TimeProvider().EpochFromSlot(futureBoundedSlotIndex) - iotago.EpochIndex(1)
		}

		reward, actualStart, actualEnd, err = deps.Protocol.MainEngineInstance().SybilProtection.DelegatorReward(
			delegationOutput.ValidatorAddress.AccountID(),
			delegationOutput.DelegatedAmount,
			delegationOutput.StartEpoch,
			delegationEnd,
		)
	}
	if err != nil {
		return nil, ierrors.Wrapf(echo.ErrInternalServerError, "failed to calculate reward for output %s: %s", outputID.ToHex(), err)
	}

	return &apimodels.ManaRewardsResponse{
		EpochStart: actualStart,
		EpochEnd:   actualEnd,
		Rewards:    reward,
	}, nil
}

func selectedCommittee(c echo.Context) (*apimodels.CommitteeResponse, error) {
	timeProvider := deps.Protocol.CommittedAPI().TimeProvider()

	var slot iotago.SlotIndex

	epoch, err := httpserver.ParseEpochQueryParam(c, restapipkg.ParameterEpochIndex)
	if err != nil {
		// by default we return current epoch
		slot = timeProvider.SlotFromTime(time.Now())
		epoch = timeProvider.EpochFromSlot(slot)
	} else {
		slot = timeProvider.EpochEnd(epoch)
	}

	seatedAccounts, exists := deps.Protocol.MainEngineInstance().SybilProtection.SeatManager().CommitteeInSlot(slot)
	if !exists {
		return &apimodels.CommitteeResponse{
			Epoch: epoch,
		}, nil
	}

	accounts, err := seatedAccounts.Accounts()
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to get accounts from committee for slot %d", slot)
	}

	committee := make([]*apimodels.CommitteeMemberResponse, 0, accounts.Size())
	accounts.ForEach(func(accountID iotago.AccountID, seat *account.Pool) bool {
		committee = append(committee, &apimodels.CommitteeMemberResponse{
			AddressBech32:  accountID.ToAddress().Bech32(deps.Protocol.CommittedAPI().ProtocolParameters().Bech32HRP()),
			PoolStake:      seat.PoolStake,
			ValidatorStake: seat.ValidatorStake,
			FixedCost:      seat.FixedCost,
		})

		return true
	})

	return &apimodels.CommitteeResponse{
		Epoch:               epoch,
		Committee:           committee,
		TotalStake:          accounts.TotalStake(),
		TotalValidatorStake: accounts.TotalValidatorStake(),
	}, nil
}
