package core

import (
	"github.com/labstack/echo/v4"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/inx-app/pkg/httpserver"
	"github.com/iotaledger/iota-core/pkg/blockhandler"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/restapi"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/nodeclient/apimodels"
)

func blockByID(c echo.Context) (*iotago.Block, error) {
	blockID, err := httpserver.ParseBlockIDParam(c, restapi.ParameterBlockID)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to parse block ID %s", c.Param(restapi.ParameterBlockID))
	}

	block, exists := deps.Protocol.MainEngineInstance().Block(blockID)
	if !exists {
		return nil, ierrors.Wrapf(echo.ErrNotFound, "block not found: %s", blockID.ToHex())
	}

	return block.ProtocolBlock(), nil
}

func blockMetadataByBlockID(blockID iotago.BlockID) (*apimodels.BlockMetadataResponse, error) {
	blockMetadata, err := deps.Protocol.MainEngineInstance().Retainer.BlockMetadata(blockID)
	if err != nil {
		return nil, ierrors.Wrapf(echo.ErrInternalServerError, "failed to get block metadata %s: %s", blockID.ToHex(), err)
	}

	return blockMetadata.BlockMetadataResponse(), nil
}

func blockMetadataByID(c echo.Context) (*apimodels.BlockMetadataResponse, error) {
	blockID, err := httpserver.ParseBlockIDParam(c, restapi.ParameterBlockID)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to parse block ID %s", c.Param(restapi.ParameterBlockID))
	}

	return blockMetadataByBlockID(blockID)
}

func blockWithMetadataByID(c echo.Context) (*apimodels.BlockWithMetadataResponse, error) {
	blockID, err := httpserver.ParseBlockIDParam(c, restapi.ParameterBlockID)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to parse block ID %s", c.Param(restapi.ParameterBlockID))
	}

	block, exists := deps.Protocol.MainEngineInstance().Block(blockID)
	if !exists {
		return nil, ierrors.Wrapf(echo.ErrNotFound, "block not found: %s", blockID.ToHex())
	}

	blockMetadata, err := blockMetadataByBlockID(blockID)
	if err != nil {
		return nil, err
	}

	return &apimodels.BlockWithMetadataResponse{
		Block:    block.ProtocolBlock(),
		Metadata: blockMetadata,
	}, nil
}

func blockIssuanceBySlot(slotIndex iotago.SlotIndex) (*apimodels.IssuanceBlockHeaderResponse, error) {
	references := deps.Protocol.MainEngineInstance().TipSelection.SelectTips(iotago.BasicBlockMaxParents)

	var slotCommitment *model.Commitment
	var err error
	// by default we use latest commitment
	if slotIndex == 0 {
		slotCommitment = deps.Protocol.MainEngineInstance().SyncManager.LatestCommitment()
	} else {
		slotCommitment, err = deps.Protocol.MainEngineInstance().Storage.Commitments().Load(slotIndex)
		if err != nil {
			return nil, ierrors.Wrapf(echo.ErrNotFound, "failed to load commitment for requested slot %d: %s", slotIndex, err)
		}
	}

	if len(references[iotago.StrongParentType]) == 0 {
		return nil, ierrors.Wrap(echo.ErrServiceUnavailable, "get references failed")
	}

	resp := &apimodels.IssuanceBlockHeaderResponse{
		StrongParents:       references[iotago.StrongParentType],
		WeakParents:         references[iotago.WeakParentType],
		ShallowLikeParents:  references[iotago.ShallowLikeParentType],
		LatestFinalizedSlot: deps.Protocol.MainEngineInstance().SyncManager.LatestFinalizedSlot(),
		Commitment:          slotCommitment.Commitment(),
	}

	return resp, nil
}

func sendBlock(c echo.Context) (*apimodels.BlockCreatedResponse, error) {
	iotaBlock, err := httpserver.ParseRequestByHeader(c, deps.Protocol.CommittedAPI(), iotago.BlockFromBytes(deps.Protocol))
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to parse iotablock")
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

	return &apimodels.BlockCreatedResponse{
		BlockID: blockID,
	}, nil
}
