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
		return nil, ierrors.Wrapf(echo.ErrBadRequest, "failed to get block ID by transaction ID: %s", err)
	}

	block, err := deps.RequestHandler.BlockByID(blockID)
	if err != nil {
		return nil, err
	}

	return block, nil
}

func blockMetadataFromTransactionID(c echo.Context) (*api.BlockMetadataResponse, error) {
	blockID, err := blockIDFromTransactionID(c)
	if err != nil {
		return nil, ierrors.Wrapf(echo.ErrBadRequest, "failed to get block ID by transaction ID: %s", err)
	}

	return deps.RequestHandler.BlockMetadataByBlockID(blockID)
}

func transactionMetadataFromTransactionID(c echo.Context) (*api.TransactionMetadataResponse, error) {
	txID, err := httpserver.ParseTransactionIDParam(c, api.ParameterTransactionID)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to parse transaction ID %s", c.Param(api.ParameterTransactionID))
	}

	return deps.RequestHandler.TransactionMetadataByID(txID)
}
