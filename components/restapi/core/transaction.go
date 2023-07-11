package core

import (
	"github.com/labstack/echo/v4"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/inx-app/pkg/httpserver"
	"github.com/iotaledger/iota-core/pkg/model"
	restapipkg "github.com/iotaledger/iota-core/pkg/restapi"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/nodeclient/models"
)

func blockIDByTransactionID(c echo.Context) (iotago.BlockID, error) {
	txID, err := httpserver.ParseTransactionIDParam(c, restapipkg.ParameterTransactionID)
	if err != nil {
		return iotago.EmptyBlockID(), err
	}

	// Get the first output of that transaction (using index 0)
	outputID := iotago.OutputID{}
	copy(outputID[:], txID[:])

	output, err := deps.Protocol.MainEngineInstance().Ledger.Output(outputID.UTXOInput())
	if err != nil {
		return iotago.EmptyBlockID(), err
	}

	return output.BlockID(), nil
}

func blockByTransactionID(c echo.Context) (*model.Block, error) {
	blockID, err := blockIDByTransactionID(c)
	if err != nil {
		return nil, err
	}

	block, exists := deps.Protocol.MainEngineInstance().Block(blockID)
	if !exists {
		return nil, ierrors.Errorf("block not found: %s", blockID.ToHex())
	}

	return block, nil
}

func blockMetadataFromTransactionID(c echo.Context) (*models.BlockMetadataResponse, error) {
	block, err := blockByTransactionID(c)
	if err != nil {
		return nil, err
	}

	// TODO: fill in blockReason, TxState, TxReason.
	bmResponse := &models.BlockMetadataResponse{
		BlockID:    block.ID().ToHex(),
		BlockState: models.BlockStatePending,
	}

	return bmResponse, nil
}
