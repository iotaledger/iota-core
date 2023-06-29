package coreapi

import (
	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"

	"github.com/iotaledger/inx-app/pkg/httpserver"
	"github.com/iotaledger/iota-core/pkg/model"
	restapipkg "github.com/iotaledger/iota-core/pkg/restapi"
	iotago "github.com/iotaledger/iota.go/v4"
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
		return nil, errors.Errorf("block not found: %s", blockID.ToHex())
	}

	return block, nil
}

func blockMetadataFromTransactionID(c echo.Context) (*blockMetadataResponse, error) {
	block, err := blockByTransactionID(c)
	if err != nil {
		return nil, err
	}

	// TODO: fill in blockReason, TxState, TxReason.
	bmResponse := &blockMetadataResponse{
		BlockID:            block.ID().ToHex(),
		StrongParents:      block.ProtocolBlock().StrongParents.ToHex(),
		WeakParents:        block.ProtocolBlock().WeakParents.ToHex(),
		ShallowLikeParents: block.ProtocolBlock().ShallowLikeParents.ToHex(),
		BlockState:         blockStatePending.String(),
	}

	return bmResponse, nil
}
