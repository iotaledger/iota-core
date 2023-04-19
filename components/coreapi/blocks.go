package coreapi

import (
	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"

	"github.com/iotaledger/inx-app/pkg/httpserver"
	"github.com/iotaledger/iota-core/pkg/restapi"
	iotago "github.com/iotaledger/iota.go/v4"
)

func blockBytesByID(c echo.Context) ([]byte, error) {
	blockID, err := httpserver.ParseBlockIDParam(c, restapi.ParameterBlockID)
	if err != nil {
		return nil, err
	}

	block, exists := deps.Protocol.MainEngineInstance().Block(blockID)
	if !exists {
		return nil, errors.Errorf("block not found: %s", blockID.ToHex())
	}
	blockBytes, err := deps.Protocol.API().Encode(block.Block)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to encode block: %s", blockID.ToHex())
	}
	return blockBytes, nil
}

func blockByID(c echo.Context) (*iotago.Block, error) {
	blockID, err := httpserver.ParseBlockIDParam(c, restapi.ParameterBlockID)
	if err != nil {
		return nil, err
	}

	block, exists := deps.Protocol.MainEngineInstance().Block(blockID)
	if !exists {
		return nil, errors.Errorf("block not found: %s", blockID.ToHex())
	}

	return block.Block(), nil
}

func blockMetadataByID(c echo.Context) (*blockMetadataResponse, error) {
	blockID, err := httpserver.ParseBlockIDParam(c, restapi.ParameterBlockID)
	if err != nil {
		return nil, err
	}
	bmResponse := &blockMetadataResponse{
		BlockID:    blockID.ToHex(),
		BlockState: blockStatePending.String(),
	}
	_, exists := deps.Protocol.MainEngineInstance().Block(blockID)
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
