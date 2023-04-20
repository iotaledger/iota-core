package coreapi

import (
	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"

	"github.com/iotaledger/inx-app/pkg/httpserver"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/restapi"
)

func blockByID(c echo.Context) (*blocks.Block, error) {
	blockID, err := httpserver.ParseBlockIDParam(c, restapi.ParameterBlockID)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse block ID: %s", c.Param(restapi.ParameterBlockID))
	}

	block, exists := deps.Protocol.MainEngineInstance().Block(blockID)
	if !exists {
		return nil, errors.Errorf("block not found: %s", blockID.ToHex())
	}

	return block, nil
}

func blockMetadataResponseByID(c echo.Context) (*blockMetadataResponse, error) {
	block, err := blockByID(c)
	if err != nil {
		return nil, err
	}
	bmResponse := &blockMetadataResponse{
		BlockID:    block.ID().ToHex(),
		BlockState: blockStatePending.String(),
	}
	_, exists := deps.Protocol.MainEngineInstance().Block(block.ID())
	if !exists {
		bmResponse.BlockError = "block not found"
	}

	// todo set states and error

	return bmResponse, nil
}

func blockIssuance(c echo.Context) (*blockIssuanceResponse, error) {
	// todo
	return nil, nil
}

func sendBlock(c echo.Context) (*blockCreatedResponse, error) {
	// todo
	return nil, nil
}
