package core

import (
	"github.com/labstack/echo/v4"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/inx-app/pkg/httpserver"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

func blockByID(c echo.Context) (*iotago.Block, error) {
	blockID, err := httpserver.ParseBlockIDParam(c, api.ParameterBlockID)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to parse block ID %s", c.Param(api.ParameterBlockID))
	}

	return deps.RequestHandler.BlockFromBlockID(blockID)
}

func blockMetadataByID(c echo.Context) (*api.BlockMetadataResponse, error) {
	blockID, err := httpserver.ParseBlockIDParam(c, api.ParameterBlockID)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to parse block ID %s", c.Param(api.ParameterBlockID))
	}

	return deps.RequestHandler.BlockMetadataFromBlockID(blockID)
}

func blockWithMetadataByID(c echo.Context) (*api.BlockWithMetadataResponse, error) {
	blockID, err := httpserver.ParseBlockIDParam(c, api.ParameterBlockID)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to parse block ID %s", c.Param(api.ParameterBlockID))
	}

	return deps.RequestHandler.BlockWithMetadataFromBlockID(blockID)
}

func sendBlock(c echo.Context) (*api.BlockCreatedResponse, error) {
	iotaBlock, err := httpserver.ParseRequestByHeader(c, deps.RequestHandler.CommittedAPI(), iotago.BlockFromBytes(deps.RequestHandler.APIProvider()))
	if err != nil {
		return nil, err
	}

	blockID, err := deps.RequestHandler.SubmitBlockAndAwaitRetainer(c.Request().Context(), iotaBlock)
	if err != nil {
		return nil, ierrors.WithMessagef(echo.ErrInternalServerError, "failed to attach block: %w", err)
	}

	return &api.BlockCreatedResponse{
		BlockID: blockID,
	}, nil
}
