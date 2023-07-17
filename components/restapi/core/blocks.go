package core

import (
	"io"

	"github.com/labstack/echo/v4"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/contextutils"
	"github.com/iotaledger/inx-app/pkg/httpserver"
	"github.com/iotaledger/iota-core/pkg/blockfactory"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/restapi"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/nodeclient/models"
)

func blockByID(c echo.Context) (*model.Block, error) {
	blockID, err := httpserver.ParseBlockIDParam(c, restapi.ParameterBlockID)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to parse block ID: %s", c.Param(restapi.ParameterBlockID))
	}

	block, err := deps.Protocol.MainEngineInstance().Retainer.Block(blockID)
	if err != nil {
		return nil, err
	}

	return block, nil
}

func blockMetadataByID(c echo.Context) (*models.BlockMetadataResponse, error) {
	block, err := blockByID(c)
	if err != nil {
		return nil, err
	}

	metadata, err := deps.Protocol.MainEngineInstance().Retainer.BlockMetadata(block.ID())
	if err != nil {
		return nil, err
	}

	// TODO: fill in blockReason, TxReason.
	bmResponse := &models.BlockMetadataResponse{
		BlockID:    block.ID().ToHex(),
		BlockState: metadata.BlockStatus.String(),
	}

	if metadata.HasTx {
		bmResponse.TxState = metadata.TransactionStatus.String()
	}

	return bmResponse, nil
}

func blockIssuance(_ echo.Context) (*models.IssuanceBlockHeaderResponse, error) {
	references := deps.Protocol.MainEngineInstance().TipSelection.SelectTips(iotago.BlockMaxParents)
	slotCommitment := deps.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment()

	if len(references[iotago.StrongParentType]) == 0 {
		return nil, ierrors.Wrap(echo.ErrServiceUnavailable, "get references failed")
	}

	resp := &models.IssuanceBlockHeaderResponse{
		StrongParents:       references[iotago.StrongParentType].ToHex(),
		WeakParents:         references[iotago.WeakParentType].ToHex(),
		ShallowLikeParents:  references[iotago.ShallowLikeParentType].ToHex(),
		LatestFinalizedSlot: deps.Protocol.MainEngineInstance().Storage.Settings().LatestFinalizedSlot(),
		Commitment:          *slotCommitment,
	}

	return resp, nil
}

func sendBlock(c echo.Context) (*models.BlockCreatedResponse, error) {
	mimeType, err := httpserver.GetRequestContentType(c, httpserver.MIMEApplicationVendorIOTASerializerV1, echo.MIMEApplicationJSON)
	if err != nil {
		return nil, err
	}

	var iotaBlock *iotago.ProtocolBlock

	if c.Request().Body == nil {
		// bad request
		return nil, ierrors.Wrap(httpserver.ErrInvalidParameter, "invalid block, error: request body missing")
	}

	bytes, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return nil, ierrors.Wrapf(httpserver.ErrInvalidParameter, "invalid block, error: %w", err)
	}

	switch mimeType {
	case echo.MIMEApplicationJSON:
		// Do not validate here, the parents might need to be set
		if err := deps.Protocol.LatestAPI().JSONDecode(bytes, iotaBlock); err != nil {
			return nil, ierrors.Wrapf(httpserver.ErrInvalidParameter, "invalid block, error: %w", err)
		}

	case httpserver.MIMEApplicationVendorIOTASerializerV1:
		// Do not validate here, the parents might need to be set
		if _, err := deps.Protocol.LatestAPI().Decode(bytes, iotaBlock); err != nil {
			return nil, ierrors.Wrapf(httpserver.ErrInvalidParameter, "invalid block, error: %w", err)
		}

	default:
		return nil, echo.ErrUnsupportedMediaType
	}

	mergedCtx, mergedCtxCancel := contextutils.MergeContexts(c.Request().Context(), Component.Daemon().ContextStopped())
	defer mergedCtxCancel()

	blockID, err := deps.BlockIssuer.AttachBlock(mergedCtx, iotaBlock)
	if err != nil {
		switch {
		case ierrors.Is(err, blockfactory.ErrBlockAttacherInvalidBlock):
			return nil, ierrors.Wrapf(httpserver.ErrInvalidParameter, "failed to attach block: %w", err)

		case ierrors.Is(err, blockfactory.ErrBlockAttacherAttachingNotPossible):
			return nil, ierrors.Wrapf(echo.ErrInternalServerError, "failed to attach block: %w", err)

		default:
			return nil, ierrors.Wrapf(echo.ErrInternalServerError, "failed to attach block: %w", err)
		}
	}

	return &models.BlockCreatedResponse{
		BlockID: blockID.ToHex(),
	}, nil
}
