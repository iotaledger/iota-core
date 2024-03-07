package core

import (
	"github.com/labstack/echo/v4"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/inx-app/pkg/httpserver"
	"github.com/iotaledger/iota-core/components/restapi"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

func congestionByAccountAddress(c echo.Context) (*api.CongestionResponse, error) {
	commitmentID, err := httpserver.ParseCommitmentIDQueryParam(c, api.ParameterCommitmentID)
	if err != nil {
		return nil, err
	}

	workScore, err := httpserver.ParseWorkScoreQueryParam(c, api.ParameterWorkScore)
	if err != nil {
		return nil, err
	}

	// if work score is 0, we don't pass it to the scheduler
	workScores := []iotago.WorkScore{}
	if workScore != 0 {
		workScores = append(workScores, workScore)
	}

	commitment, err := deps.RequestHandler.GetCommitmentByID(commitmentID)
	if err != nil {
		return nil, err
	}

	hrp := deps.RequestHandler.CommittedAPI().ProtocolParameters().Bech32HRP()
	address, err := httpserver.ParseBech32AddressParam(c, hrp, api.ParameterBech32Address)
	if err != nil {
		return nil, err
	}

	accountAddress, ok := address.(*iotago.AccountAddress)
	if !ok {
		return nil, ierrors.Wrapf(httpserver.ErrInvalidParameter, "address %s is not an account address", c.Param(api.ParameterBech32Address))
	}

	return deps.RequestHandler.CongestionByAccountAddress(accountAddress, commitment, workScores...)
}

func validators(c echo.Context) (*api.ValidatorsResponse, error) {
	var err error
	pageSize := httpserver.ParsePageSizeQueryParam(c, api.ParameterPageSize, restapi.ParamsRestAPI.MaxPageSize)
	latestCommittedSlot := deps.RequestHandler.GetLatestCommitment().Slot()

	// no cursor provided will be the first request
	requestedSlot := latestCommittedSlot
	var cursorIndex uint32
	if len(c.QueryParam(api.ParameterCursor)) != 0 {
		requestedSlot, cursorIndex, err = httpserver.ParseSlotCursorQueryParam(c, api.ParameterCursor)
		if err != nil {
			return nil, err
		}
	}

	// do not respond to really old requests
	if requestedSlot+iotago.SlotIndex(restapi.ParamsRestAPI.MaxRequestedSlotAge) < latestCommittedSlot {
		return nil, ierrors.Wrapf(echo.ErrBadRequest, "request is too old, request started at %d, latest committed slot index is %d", requestedSlot, latestCommittedSlot)
	}
	slotRange := uint32(requestedSlot) / restapi.ParamsRestAPI.RequestsMemoryCacheGranularity

	return deps.RequestHandler.Validators(slotRange, pageSize, cursorIndex)
}

func validatorByAccountAddress(c echo.Context) (*api.ValidatorResponse, error) {
	hrp := deps.RequestHandler.CommittedAPI().ProtocolParameters().Bech32HRP()
	address, err := httpserver.ParseBech32AddressParam(c, hrp, api.ParameterBech32Address)
	if err != nil {
		return nil, err
	}

	accountAddress, ok := address.(*iotago.AccountAddress)
	if !ok {
		return nil, ierrors.Wrapf(httpserver.ErrInvalidParameter, "address %s is not an account address", c.Param(api.ParameterBech32Address))
	}

	return deps.RequestHandler.ValidatorByAccountAddress(accountAddress)
}

func rewardsByOutputID(c echo.Context) (*api.ManaRewardsResponse, error) {
	outputID, err := httpserver.ParseOutputIDParam(c, api.ParameterOutputID)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to parse output ID %s", c.Param(api.ParameterOutputID))
	}

	var slot iotago.SlotIndex
	if len(c.QueryParam(api.ParameterSlot)) > 0 {
		var err error
		slot, err = httpserver.ParseSlotQueryParam(c, api.ParameterSlot)
		if err != nil {
			return nil, ierrors.Wrapf(err, "failed to parse slot index %s", c.Param(api.ParameterSlot))
		}
		genesisSlot := deps.RequestHandler.LatestAPI().ProtocolParameters().GenesisSlot()
		if slot < genesisSlot {
			return nil, ierrors.Wrapf(echo.ErrBadRequest, "slot index (%d) before genesis slot (%d)", slot, genesisSlot)
		}
	} else {
		// The slot index may be unset for requests that do not want to issue a transaction, such as displaying estimated rewards,
		// in which case we use latest committed slot.
		slot = deps.RequestHandler.GetLatestCommitment().Slot()
	}

	return deps.RequestHandler.RewardsByOutputID(outputID, slot)
}

func selectedCommittee(c echo.Context) (*api.CommitteeResponse, error) {
	var epoch iotago.EpochIndex
	if len(c.QueryParam(api.ParameterEpoch)) == 0 {
		// by default we return current epoch
		epoch = deps.RequestHandler.CommittedAPI().TimeProvider().CurrentEpoch()
	} else {
		var err error
		epoch, err = httpserver.ParseEpochQueryParam(c, api.ParameterEpoch)
		if err != nil {
			return nil, err
		}
	}

	return deps.RequestHandler.SelectedCommittee(epoch)
}
