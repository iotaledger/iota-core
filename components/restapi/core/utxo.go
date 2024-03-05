package core

import (
	"github.com/labstack/echo/v4"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/inx-app/pkg/httpserver"
	"github.com/iotaledger/iota.go/v4/api"
)

func outputFromOutputID(c echo.Context) (*api.OutputResponse, error) {
	outputID, err := httpserver.ParseOutputIDParam(c, api.ParameterOutputID)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to parse output ID %s", c.Param(api.ParameterOutputID))
	}

	return deps.RequestHandler.OutputFromOutputID(outputID)
}

func outputMetadataFromOutputID(c echo.Context) (*api.OutputMetadata, error) {
	outputID, err := httpserver.ParseOutputIDParam(c, api.ParameterOutputID)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to parse output ID %s", c.Param(api.ParameterOutputID))
	}

	return deps.RequestHandler.OutputMetadataFromOutputID(outputID)
}

func outputWithMetadataFromOutputID(c echo.Context) (*api.OutputWithMetadataResponse, error) {
	outputID, err := httpserver.ParseOutputIDParam(c, api.ParameterOutputID)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to parse output ID %s", c.Param(api.ParameterOutputID))
	}

	return deps.RequestHandler.OutputWithMetadataFromOutputID(outputID)
}
