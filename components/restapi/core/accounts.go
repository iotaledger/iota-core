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

	hrp := deps.RequestHandler.CommittedAPI().ProtocolParameters().Bech32HRP()
	address, err := httpserver.ParseBech32AddressParam(c, hrp, api.ParameterBech32Address)
	if err != nil {
		return nil, err
	}

	accountAddress, ok := address.(*iotago.AccountAddress)
	if !ok {
		return nil, ierrors.Wrapf(httpserver.ErrInvalidParameter, "address %s is not an account address", c.Param(api.ParameterBech32Address))
	}

	return deps.RequestHandler.CongestionByAccountAddress(accountAddress, workScore, commitmentID)
}

func validators(c echo.Context) (*api.ValidatorsResponse, error) {
	var err error
	pageSize := httpserver.ParsePageSizeQueryParam(c, api.ParameterPageSize, restapi.ParamsRestAPI.MaxPageSize)
	latestCommittedSlot := deps.RequestHandler.GetLatestCommitment().Slot()
	currentEpoch := deps.RequestHandler.APIProvider().APIForSlot(latestCommittedSlot).TimeProvider().EpochFromSlot(latestCommittedSlot)
	requestedEpoch := currentEpoch

	// no cursor provided will be the first request
	var cursorIndex uint32
	if len(c.QueryParam(api.ParameterCursor)) != 0 {
		requestedEpoch, cursorIndex, err = httpserver.ParseEpochCursorQueryParam(c, api.ParameterCursor)
		if err != nil {
			return nil, err
		}
	}

	if requestedEpoch > currentEpoch || requestedEpoch <= deps.RequestHandler.GetNodeStatus().PruningEpoch {
		return nil, ierrors.Wrapf(echo.ErrBadRequest, "epoch %d is larger than current epoch or already pruned", requestedEpoch)
	}

	return deps.RequestHandler.Validators(requestedEpoch, cursorIndex, pageSize)
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
	var err error
	outputID, err := httpserver.ParseOutputIDParam(c, api.ParameterOutputID)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to parse output ID %s", c.Param(api.ParameterOutputID))
	}

	var slot []iotago.SlotIndex
	if len(c.QueryParam(api.ParameterSlot)) > 0 {
		slotParam, err := httpserver.ParseSlotQueryParam(c, api.ParameterSlot)
		if err != nil {
			return nil, ierrors.Wrapf(err, "failed to parse slot index %s", c.Param(api.ParameterSlot))
		}
		slot = append(slot, slotParam)
	}

	return deps.RequestHandler.RewardsByOutputID(outputID, slot...)
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
		currentEpoch := deps.RequestHandler.CommittedAPI().TimeProvider().CurrentEpoch()
		if epoch > currentEpoch {
			return nil, ierrors.Wrapf(echo.ErrBadRequest, "provided epoch %d is from the future, current epoch: %d", epoch, currentEpoch)
		}
	}

	return deps.RequestHandler.SelectedCommittee(epoch)
}
