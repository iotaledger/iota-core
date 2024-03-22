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
		return nil, ierrors.Wrap(err, "failed to get block ID by transaction ID")
	}

	return deps.RequestHandler.BlockFromBlockID(blockID)
}

func blockMetadataFromTransactionID(c echo.Context) (*api.BlockMetadataResponse, error) {
	blockID, err := blockIDFromTransactionID(c)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to get block ID by transaction ID")
	}

	return deps.RequestHandler.BlockMetadataFromBlockID(blockID)
}

func transactionFromTransactionID(c echo.Context) (*iotago.Transaction, error) {
	txID, err := httpserver.ParseTransactionIDParam(c, api.ParameterTransactionID)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to parse transaction ID")
	}

	blockID, err := deps.RequestHandler.BlockIDFromTransactionID(txID)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to get block ID by transaction ID")
	}

	block, err := deps.RequestHandler.ModelBlockFromBlockID(blockID)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to get block by block ID")
	}

	tx, isTransaction := block.SignedTransaction()
	if !isTransaction {
		return nil, ierrors.WithMessagef(echo.ErrInternalServerError, "block %s does not contain a transaction", blockID)
	}

	return tx.Transaction, nil
}

func transactionMetadataFromTransactionID(c echo.Context) (*api.TransactionMetadataResponse, error) {
	txID, err := httpserver.ParseTransactionIDParam(c, api.ParameterTransactionID)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to parse transaction ID")
	}

	return deps.RequestHandler.TransactionMetadataFromTransactionID(txID)
}
