package core

import (
	"github.com/labstack/echo/v4"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/inx-app/pkg/httpserver"
	"github.com/iotaledger/iota-core/pkg/requesthandler"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

func blockByID(c echo.Context) (*iotago.Block, error) {
	blockID, err := httpserver.ParseBlockIDParam(c, api.ParameterBlockID)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to parse block ID %s", c.Param(api.ParameterBlockID))
	}

	return deps.RequestHandler.BlockByID(blockID)
}

func blockMetadataByID(c echo.Context) (*api.BlockMetadataResponse, error) {
	blockID, err := httpserver.ParseBlockIDParam(c, api.ParameterBlockID)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to parse block ID %s", c.Param(api.ParameterBlockID))
	}

	return deps.RequestHandler.BlockMetadataByBlockID(blockID)
}

func blockWithMetadataByID(c echo.Context) (*api.BlockWithMetadataResponse, error) {
	blockID, err := httpserver.ParseBlockIDParam(c, api.ParameterBlockID)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to parse block ID %s", c.Param(api.ParameterBlockID))
	}

	return deps.RequestHandler.BlockWithMetadataByID(blockID)
}

func sendBlock(c echo.Context) (*api.BlockCreatedResponse, error) {
	iotaBlock, err := httpserver.ParseRequestByHeader(c, deps.RequestHandler.CommittedAPI(), iotago.BlockFromBytes(deps.RequestHandler.APIProvider()))
	if err != nil {
		return nil, err
	}

	blockID, err := deps.RequestHandler.SubmitBlockAndAwaitBooking(c.Request().Context(), iotaBlock)
	if err != nil {
		switch {
		case ierrors.Is(err, requesthandler.ErrBlockAttacherInvalidBlock):
			return nil, ierrors.Wrapf(httpserver.ErrInvalidParameter, "failed to attach block: %w", err)

		case ierrors.Is(err, requesthandler.ErrBlockAttacherAttachingNotPossible):
			return nil, ierrors.Wrapf(echo.ErrInternalServerError, "failed to attach block: %w", err)

		default:
			return nil, ierrors.Wrapf(echo.ErrInternalServerError, "failed to attach block: %w", err)
		}
	}

	return &api.BlockCreatedResponse{
		BlockID: blockID,
	}, nil
}
