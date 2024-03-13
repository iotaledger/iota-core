package requesthandler

import (
	"fmt"
	"sort"

	"github.com/labstack/echo/v4"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/serializer/v2/serix"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/model"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

var validatorResponsesTypeSettings = serix.TypeSettings{}.WithLengthPrefixType(serix.LengthPrefixTypeAsByte)

func (r *RequestHandler) CongestionByAccountAddress(accountAddress *iotago.AccountAddress, commitment *model.Commitment, workScores ...iotago.WorkScore) (*api.CongestionResponse, error) {
	accountID := accountAddress.AccountID()
	acc, exists, err := r.protocol.Engines.Main.Get().Ledger.Account(accountID, commitment.Slot())
	if err != nil {
		return nil, ierrors.Wrapf(echo.ErrInternalServerError, "failed to get account %s from the Ledger: %s", accountID.ToHex(), err)
	}
	if !exists {
		return nil, ierrors.Wrapf(echo.ErrNotFound, "account not found: %s", accountID.ToHex())
	}

	return &api.CongestionResponse{
		Slot:                 commitment.Slot(),
		Ready:                r.protocol.Engines.Main.Get().Scheduler.IsBlockIssuerReady(accountID, workScores...),
		ReferenceManaCost:    commitment.ReferenceManaCost(),
		BlockIssuanceCredits: acc.Credits.Value,
	}, nil
}

func (r *RequestHandler) registeredValidatorsFromCache(epochIndex iotago.EpochIndex, key []byte) ([]*api.ValidatorResponse, error) {
	apiForEpoch := r.APIProvider().APIForEpoch(epochIndex)

	registeredValidatorsBytes := r.registeredValidatorsCache.Get(key)
	if registeredValidatorsBytes == nil {
		// get the ordered registered validators list from engine.
		registeredValidators, err := r.protocol.Engines.Main.Get().SybilProtection.OrderedRegisteredCandidateValidatorsList(epochIndex)
		if err != nil {
			return nil, ierrors.Wrapf(echo.ErrNotFound, " ordered registered validators list for epoch %d not found: %s", epochIndex, err)
		}

		// store validator responses in cache.
		registeredValidatorsBytes, err := apiForEpoch.Encode(registeredValidators, serix.WithTypeSettings(validatorResponsesTypeSettings))
		if err != nil {
			return nil, ierrors.Wrapf(echo.ErrInternalServerError, "failed to encode ordered registered validators list for epoch %d : %s", epochIndex, err)
		}
		r.registeredValidatorsCache.Set(key, registeredValidatorsBytes)

		return registeredValidators, nil
	}

	validatorResp := make([]*api.ValidatorResponse, 0)
	_, err := apiForEpoch.Decode(registeredValidatorsBytes, &validatorResp, serix.WithTypeSettings(validatorResponsesTypeSettings))
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to decode validator responses for epoch %d", epochIndex)
	}

	return validatorResp, nil
}

func (r *RequestHandler) Validators(epochIndex iotago.EpochIndex, cursorIndex, pageSize uint32) (*api.ValidatorsResponse, error) {
	apiForEpoch := r.APIProvider().APIForEpoch(epochIndex)
	currentSlot := r.protocol.Engines.Main.Get().SyncManager.LatestCommitment().Slot()
	currentEpoch := apiForEpoch.TimeProvider().EpochFromSlot(currentSlot)
	var registeredValidators []*api.ValidatorResponse
	var err error

	if epochIndex == currentEpoch {
		// The key of validators cache is the combination of current epoch and current slot. So the results is updated when new commitment is created.
		key := append(currentEpoch.MustBytes(), currentSlot.MustBytes()...)
		registeredValidators, err = r.registeredValidatorsFromCache(epochIndex, key)
	} else {
		registeredValidators, err = r.registeredValidatorsFromCache(epochIndex, epochIndex.MustBytes())
	}
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to get registered validators for epoch %d", epochIndex)
	}

	pageEndIndex := lo.Min(cursorIndex+pageSize, uint32(len(registeredValidators)))
	if cursorIndex >= pageEndIndex {
		return nil, ierrors.Wrapf(echo.ErrBadRequest, "invalid pagination cursorIndex, requesting index %d to %d", cursorIndex, pageEndIndex)
	}

	page := registeredValidators[cursorIndex:pageEndIndex]
	resp := &api.ValidatorsResponse{
		Validators: page,
		PageSize:   pageSize,
	}
	// this is the last page
	if int(cursorIndex+pageSize) > len(registeredValidators) {
		resp.Cursor = ""
	} else {
		resp.Cursor = fmt.Sprintf("%d,%d", epochIndex, cursorIndex+pageSize)
	}

	return resp, nil
}

func (r *RequestHandler) ValidatorByAccountAddress(accountAddress *iotago.AccountAddress) (*api.ValidatorResponse, error) {
	latestCommittedSlot := r.protocol.Engines.Main.Get().SyncManager.LatestCommitment().Slot()

	accountID := accountAddress.AccountID()
	accountData, exists, err := r.protocol.Engines.Main.Get().Ledger.Account(accountID, latestCommittedSlot)
	if err != nil {
		return nil, ierrors.Wrapf(echo.ErrInternalServerError, "failed to get account %s from the Ledger: %s", accountID.ToHex(), err)
	}
	if !exists {
		return nil, ierrors.Wrapf(echo.ErrNotFound, "account %s not found for latest committedSlot %d", accountID.ToHex(), latestCommittedSlot)
	}

	epoch := r.protocol.APIForSlot(latestCommittedSlot).TimeProvider().EpochFromSlot(latestCommittedSlot)

	active, err := r.protocol.Engines.Main.Get().SybilProtection.IsCandidateActive(accountID, epoch)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to check if account %s is an active candidate", accountID.ToHex())
	}

	return &api.ValidatorResponse{
		AddressBech32:                  accountID.ToAddress().Bech32(r.protocol.CommittedAPI().ProtocolParameters().Bech32HRP()),
		PoolStake:                      accountData.ValidatorStake + accountData.DelegationStake,
		ValidatorStake:                 accountData.ValidatorStake,
		StakingEndEpoch:                accountData.StakeEndEpoch,
		FixedCost:                      accountData.FixedCost,
		Active:                         active,
		LatestSupportedProtocolVersion: accountData.LatestSupportedProtocolVersionAndHash.Version,
		LatestSupportedProtocolHash:    accountData.LatestSupportedProtocolVersionAndHash.Hash,
	}, nil
}

func (r *RequestHandler) RewardsByOutputID(outputID iotago.OutputID, slot iotago.SlotIndex) (*api.ManaRewardsResponse, error) {
	utxoOutput, err := r.protocol.Engines.Main.Get().Ledger.Output(outputID)
	if err != nil {
		if ierrors.Is(err, kvstore.ErrKeyNotFound) {
			return nil, ierrors.Wrapf(echo.ErrNotFound, "output %s not found", outputID.ToHex())
		}

		return nil, ierrors.Wrapf(echo.ErrInternalServerError, "failed to get output %s from ledger: %s", outputID.ToHex(), err)
	}

	var stakingPoolValidatorAccountID iotago.AccountID
	var reward iotago.Mana
	var firstRewardEpoch, lastRewardEpoch iotago.EpochIndex

	apiForSlot := r.protocol.APIForSlot(slot)

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

		futureBoundedSlotIndex := slot + apiForSlot.ProtocolParameters().MinCommittableAge()
		claimingEpoch := apiForSlot.TimeProvider().EpochFromSlot(futureBoundedSlotIndex)

		stakingPoolValidatorAccountID = accountOutput.AccountID
		// check if the account is a validator
		reward, firstRewardEpoch, lastRewardEpoch, err = r.protocol.Engines.Main.Get().SybilProtection.ValidatorReward(
			stakingPoolValidatorAccountID,
			stakingFeature,
			claimingEpoch,
		)

	case iotago.OutputDelegation:
		//nolint:forcetypeassert
		delegationOutput := utxoOutput.Output().(*iotago.DelegationOutput)
		delegationEnd := delegationOutput.EndEpoch
		futureBoundedSlotIndex := slot + apiForSlot.ProtocolParameters().MinCommittableAge()
		claimingEpoch := apiForSlot.TimeProvider().EpochFromSlot(futureBoundedSlotIndex)

		// If Delegation ID is zeroed, the output is in delegating state, which means its End Epoch is not set and we must use the
		// "last epoch" for the rewards calculation.
		// In this case the calculation must be consistent with the rewards calculation at execution time, so a client can specify
		// a slot index explicitly, which should be equal to the slot it uses as the commitment input for the claiming transaction.
		if delegationOutput.DelegationID.Empty() {
			delegationEnd = claimingEpoch - iotago.EpochIndex(1)
		}

		stakingPoolValidatorAccountID = delegationOutput.ValidatorAddress.AccountID()

		reward, firstRewardEpoch, lastRewardEpoch, err = r.protocol.Engines.Main.Get().SybilProtection.DelegatorReward(
			stakingPoolValidatorAccountID,
			delegationOutput.DelegatedAmount,
			delegationOutput.StartEpoch,
			delegationEnd,
			claimingEpoch,
		)
	default:
		return nil, ierrors.Wrapf(echo.ErrBadRequest, "output %s is neither a delegation output nor account", outputID.ToHex())
	}

	if err != nil {
		return nil, ierrors.Wrapf(echo.ErrInternalServerError, "failed to calculate reward for output %s: %s", outputID.ToHex(), err)
	}

	latestCommittedEpochPoolRewards, poolRewardExists, err := r.protocol.Engines.Main.Get().SybilProtection.PoolRewardsForAccount(stakingPoolValidatorAccountID)
	if err != nil {
		return nil, ierrors.Wrapf(echo.ErrInternalServerError, "failed to retrieve pool rewards for account %s: %s", stakingPoolValidatorAccountID.ToHex(), err)
	}

	if !poolRewardExists {
		latestCommittedEpochPoolRewards = 0
	}

	return &api.ManaRewardsResponse{
		StartEpoch:                      firstRewardEpoch,
		EndEpoch:                        lastRewardEpoch,
		Rewards:                         reward,
		LatestCommittedEpochPoolRewards: latestCommittedEpochPoolRewards,
	}, nil
}

func (r *RequestHandler) SelectedCommittee(epoch iotago.EpochIndex) (*api.CommitteeResponse, error) {
	seatedAccounts, exists := r.protocol.Engines.Main.Get().SybilProtection.SeatManager().CommitteeInEpoch(epoch)
	if !exists {
		return &api.CommitteeResponse{
			Epoch: epoch,
		}, nil
	}

	accounts, err := seatedAccounts.Accounts()
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to get accounts from committee for epoch %d", epoch)
	}

	committee := make([]*api.CommitteeMemberResponse, 0, accounts.Size())
	accounts.ForEach(func(accountID iotago.AccountID, seat *account.Pool) bool {
		committee = append(committee, &api.CommitteeMemberResponse{
			AddressBech32:  accountID.ToAddress().Bech32(r.protocol.CommittedAPI().ProtocolParameters().Bech32HRP()),
			PoolStake:      seat.PoolStake,
			ValidatorStake: seat.ValidatorStake,
			FixedCost:      seat.FixedCost,
		})

		return true
	})

	// sort committee by pool stake, then by address
	sort.Slice(committee, func(i, j int) bool {
		if committee[i].PoolStake == committee[j].PoolStake {
			return committee[i].AddressBech32 < committee[j].AddressBech32
		}

		return committee[i].PoolStake > committee[j].PoolStake
	})

	return &api.CommitteeResponse{
		Epoch:               epoch,
		Committee:           committee,
		TotalStake:          accounts.TotalStake(),
		TotalValidatorStake: accounts.TotalValidatorStake(),
	}, nil
}
