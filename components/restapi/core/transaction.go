package core

import (
	"github.com/labstack/echo/v4"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/inx-app/pkg/httpserver"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

func blockIDFromTransactionID(c echo.Context) (iotago.BlockID, error) {
	txID, err := httpserver.ParseTransactionIDParam(c, api.ParameterTransactionID)
	if err != nil {
		return iotago.EmptyBlockID, ierrors.Wrapf(err, "failed to parse transaction ID %s", c.Param(api.ParameterTransactionID))
	}

	return deps.RequestHandler.BlockIDFromTransactionID(txID)
}

func blockFromTransactionID(c echo.Context) (*iotago.Block, error) {
	blockID, err := blockIDFromTransactionID(c)
	if err != nil {
		return nil, ierrors.WithMessagef(echo.ErrBadRequest, "failed to get block ID by transaction ID: %w", err)
	}

	return deps.RequestHandler.BlockFromBlockID(blockID)
}

func blockMetadataFromTransactionID(c echo.Context) (*api.BlockMetadataResponse, error) {
	blockID, err := blockIDFromTransactionID(c)
	if err != nil {
		return nil, ierrors.WithMessagef(echo.ErrBadRequest, "failed to get block ID by transaction ID: %w", err)
	}

	return deps.RequestHandler.BlockMetadataFromBlockID(blockID)
}

func transactionMetadataFromTransactionID(c echo.Context) (*api.TransactionMetadataResponse, error) {
	txID, err := httpserver.ParseTransactionIDParam(c, api.ParameterTransactionID)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to parse transaction ID %s", c.Param(api.ParameterTransactionID))
	}

	return deps.RequestHandler.TransactionMetadataFromTransactionID(txID)
}
