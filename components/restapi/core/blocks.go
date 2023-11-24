package core

import (
	"github.com/labstack/echo/v4"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/inx-app/pkg/httpserver"
	"github.com/iotaledger/iota-core/pkg/blockhandler"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

func blockByID(c echo.Context) (*iotago.Block, error) {
	blockID, err := httpserver.ParseBlockIDParam(c, api.ParameterBlockID)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to parse block ID %s", c.Param(api.ParameterBlockID))
	}

	block, exists := deps.Protocol.Engines.Main.Get().Block(blockID)
	if !exists {
		return nil, ierrors.Wrapf(echo.ErrNotFound, "block not found: %s", blockID.ToHex())
	}

	return block.ProtocolBlock(), nil
}

func blockMetadataByBlockID(blockID iotago.BlockID) (*api.BlockMetadataResponse, error) {
	blockMetadata, err := deps.Protocol.Engines.Main.Get().Retainer.BlockMetadata(blockID)
	if err != nil {
		return nil, ierrors.Wrapf(echo.ErrInternalServerError, "failed to get block metadata %s: %s", blockID.ToHex(), err)
	}

	return blockMetadata.BlockMetadataResponse(), nil
}

func blockMetadataByID(c echo.Context) (*api.BlockMetadataResponse, error) {
	blockID, err := httpserver.ParseBlockIDParam(c, api.ParameterBlockID)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to parse block ID %s", c.Param(api.ParameterBlockID))
	}

	return blockMetadataByBlockID(blockID)
}

func blockWithMetadataByID(c echo.Context) (*api.BlockWithMetadataResponse, error) {
	blockID, err := httpserver.ParseBlockIDParam(c, api.ParameterBlockID)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to parse block ID %s", c.Param(api.ParameterBlockID))
	}

	block, exists := deps.Protocol.Engines.Main.Get().Block(blockID)
	if !exists {
		return nil, ierrors.Wrapf(echo.ErrNotFound, "block not found: %s", blockID.ToHex())
	}

	blockMetadata, err := blockMetadataByBlockID(blockID)
	if err != nil {
		return nil, err
	}

	return &api.BlockWithMetadataResponse{
		Block:    block.ProtocolBlock(),
		Metadata: blockMetadata,
	}, nil
}

func blockIssuance() (*api.IssuanceBlockHeaderResponse, error) {
	references := deps.Protocol.Engines.Main.Get().TipSelection.SelectTips(iotago.BasicBlockMaxParents)
	if len(references[iotago.StrongParentType]) == 0 {
		return nil, ierrors.Wrap(echo.ErrServiceUnavailable, "no strong parents available")
	}

	resp := &api.IssuanceBlockHeaderResponse{
		StrongParents:       references[iotago.StrongParentType],
		WeakParents:         references[iotago.WeakParentType],
		ShallowLikeParents:  references[iotago.ShallowLikeParentType],
		LatestFinalizedSlot: deps.Protocol.Engines.Main.Get().SyncManager.LatestFinalizedSlot(),
		Commitment:          deps.Protocol.Engines.Main.Get().SyncManager.LatestCommitment().Commitment(),
	}

	return resp, nil
}

func sendBlock(c echo.Context) (*api.BlockCreatedResponse, error) {
	iotaBlock, err := httpserver.ParseRequestByHeader(c, deps.Protocol.CommittedAPI(), iotago.BlockFromBytes(deps.Protocol))
	if err != nil {
		return nil, err
	}

	blockID, err := deps.BlockHandler.AttachBlock(c.Request().Context(), iotaBlock)
	if err != nil {
		switch {
		case ierrors.Is(err, blockhandler.ErrBlockAttacherInvalidBlock):
			return nil, ierrors.Wrapf(httpserver.ErrInvalidParameter, "failed to attach block: %w", err)

		case ierrors.Is(err, blockhandler.ErrBlockAttacherAttachingNotPossible):
			return nil, ierrors.Wrapf(echo.ErrInternalServerError, "failed to attach block: %w", err)

		default:
			return nil, ierrors.Wrapf(echo.ErrInternalServerError, "failed to attach block: %w", err)
		}
	}

	return &api.BlockCreatedResponse{
		BlockID: blockID,
	}, nil
}
