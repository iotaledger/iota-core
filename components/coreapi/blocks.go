package coreapi

import (
	"io"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/runtime/contextutils"
	"github.com/iotaledger/inx-app/pkg/httpserver"
	"github.com/iotaledger/iota-core/pkg/blockissuer"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/restapi"
	iotago "github.com/iotaledger/iota.go/v4"
)

func blockByID(c echo.Context) (*model.Block, error) {
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

	// TODO: fill in blockReason, TxState, TxReason.
	bmResponse := &blockMetadataResponse{
		BlockID:            block.ID().ToHex(),
		StrongParents:      block.Block().StrongParents.ToHex(),
		WeakParents:        block.Block().WeakParents.ToHex(),
		ShallowLikeParents: block.Block().ShallowLikeParents.ToHex(),
		BlockState:         blockStatePending.String(),
	}

	return bmResponse, nil
}

func blockIssuance(_ echo.Context) (*blockIssuanceResponse, error) {
	references := deps.Protocol.MainEngineInstance().TipSelection.SelectTips(iotago.BlockMaxParents)
	slotCommitment := deps.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment()

	if len(references[model.StrongParentType]) == 0 {
		return nil, errors.Wrapf(echo.ErrServiceUnavailable, "get references failed")
	}

	cBytes, err := deps.Protocol.API().JSONEncode(slotCommitment.Commitment())
	if err != nil {
		return nil, err
	}

	resp := &blockIssuanceResponse{
		StrongParents:       references[model.StrongParentType].ToHex(),
		WeakParents:         references[model.WeakParentType].ToHex(),
		ShallowLikeParents:  references[model.ShallowLikeParentType].ToHex(),
		LatestFinalizedSlot: deps.Protocol.MainEngineInstance().Storage.Settings().LatestFinalizedSlot(),
		Commitment:          cBytes,
	}

	return resp, nil
}

func sendBlock(c echo.Context) (*blockCreatedResponse, error) {
	mimeType, err := httpserver.GetRequestContentType(c, httpserver.MIMEApplicationVendorIOTASerializerV1, echo.MIMEApplicationJSON)
	if err != nil {
		return nil, err
	}

	iotaBlock := &iotago.Block{
		Signature: &iotago.Ed25519Signature{},
	}

	switch mimeType {
	case echo.MIMEApplicationJSON:
		if err := c.Bind(iotaBlock); err != nil {
			return nil, errors.WithMessagef(httpserver.ErrInvalidParameter, "invalid block, error: %s", err)
		}

	case httpserver.MIMEApplicationVendorIOTASerializerV1:
		if c.Request().Body == nil {
			// bad request
			return nil, errors.WithMessage(httpserver.ErrInvalidParameter, "invalid block, error: request body missing")
		}

		bytes, err := io.ReadAll(c.Request().Body)
		if err != nil {
			return nil, errors.WithMessagef(httpserver.ErrInvalidParameter, "invalid block, error: %s", err)
		}

		// Do not validate here, the parents might need to be set
		if _, err := deps.Protocol.API().Decode(bytes, iotaBlock); err != nil {
			return nil, errors.WithMessagef(httpserver.ErrInvalidParameter, "invalid block, error: %s", err)
		}

	default:
		return nil, echo.ErrUnsupportedMediaType
	}

	mergedCtx, mergedCtxCancel := contextutils.MergeContexts(c.Request().Context(), Component.Daemon().ContextStopped())
	defer mergedCtxCancel()

	blockID, err := deps.BlockIssuer.AttachBlock(mergedCtx, iotaBlock)
	if err != nil {
		switch {
		case errors.Is(err, blockissuer.ErrBlockAttacherInvalidBlock):
			return nil, errors.WithMessagef(httpserver.ErrInvalidParameter, "failed to attach block: %s", err.Error())

		case errors.Is(err, blockissuer.ErrBlockAttacherAttachingNotPossible):
			return nil, errors.WithMessagef(echo.ErrInternalServerError, "failed to attach block: %s", err.Error())

		case errors.Is(err, blockissuer.ErrBlockAttacherPoWNotAvailable):
			return nil, errors.WithMessagef(echo.ErrServiceUnavailable, "failed to attach block: %s", err.Error())

		default:
			return nil, errors.WithMessagef(echo.ErrInternalServerError, "failed to attach block: %s", err.Error())
		}
	}

	return &blockCreatedResponse{
		BlockID: blockID.ToHex(),
	}, nil
}
