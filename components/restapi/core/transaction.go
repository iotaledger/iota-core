package core

import (
	"github.com/labstack/echo/v4"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/inx-app/pkg/httpserver"
	"github.com/iotaledger/iota-core/pkg/model"
	restapipkg "github.com/iotaledger/iota-core/pkg/restapi"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/nodeclient/apimodels"
)

func blockIDByTransactionID(c echo.Context) (iotago.BlockID, error) {
	txID, err := httpserver.ParseTransactionIDParam(c, restapipkg.ParameterTransactionID)
	if err != nil {
		return iotago.EmptyBlockID, ierrors.Wrapf(err, "failed to parse transaction ID: %s", c.Param(restapipkg.ParameterTransactionID))
	}

	return blockIDFromTransactionID(txID)
}

func blockIDFromTransactionID(transactionID iotago.TransactionID) (iotago.BlockID, error) {
	// Get the first output of that transaction (using index 0)
	outputID := iotago.OutputIDFromTransactionIDAndIndex(transactionID, 0)

	output, spent, err := deps.Protocol.Engines.Main.Get().Ledger.OutputOrSpent(outputID)
	if err != nil {
		return iotago.EmptyBlockID, ierrors.Wrapf(err, "failed to get output: %s", outputID.ToHex())
	}

	if output != nil {
		return output.BlockID(), nil
	}

	return spent.BlockID(), nil
}

func blockByTransactionID(c echo.Context) (*model.Block, error) {
	blockID, err := blockIDByTransactionID(c)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to get block ID by transaction ID")
	}

	block, exists := deps.Protocol.Engines.Main.Get().Block(blockID)
	if !exists {
		return nil, ierrors.Errorf("block not found: %s", blockID.String())
	}

	return block, nil
}

func blockMetadataFromTransactionID(c echo.Context) (*apimodels.BlockMetadataResponse, error) {
	blockID, err := blockIDByTransactionID(c)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to get block ID by transaction ID")
	}

	return blockMetadataByBlockID(blockID)
}
