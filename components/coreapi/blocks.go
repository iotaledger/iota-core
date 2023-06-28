package coreapi

import (
	"encoding/json"
	"io"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/runtime/contextutils"
	"github.com/iotaledger/inx-app/pkg/httpserver"
	"github.com/iotaledger/iota-core/pkg/blockfactory"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/restapi"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/nodeclient"
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

func blockMetadataResponseByID(c echo.Context) (*nodeclient.BlockMetadataResponse, error) {
	block, err := blockByID(c)
	if err != nil {
		return nil, err
	}

	// TODO: fill in blockReason, TxState, TxReason.
	bmResponse := &nodeclient.BlockMetadataResponse{
		BlockID:            block.ID().ToHex(),
		StrongParents:      block.Block().StrongParents.ToHex(),
		WeakParents:        block.Block().WeakParents.ToHex(),
		ShallowLikeParents: block.Block().ShallowLikeParents.ToHex(),
		BlockState:         blockStatePending.String(),
	}

	return bmResponse, nil
}

func blockIssuance(_ echo.Context) (*nodeclient.BlockIssuanceResponse, error) {
	references := deps.Protocol.MainEngineInstance().TipManager.SelectTips(iotago.BlockMaxParents)
	slotCommitment := deps.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment()

	if len(references[model.StrongParentType]) == 0 {
		return nil, errors.Wrap(echo.ErrServiceUnavailable, "get references failed")
	}

	cBytes, err := deps.Protocol.API().JSONEncode(slotCommitment.Commitment())
	if err != nil {
		return nil, err
	}
	commitmentJSONRaw := json.RawMessage(cBytes)

	resp := &nodeclient.BlockIssuanceResponse{
		StrongParents:       references[model.StrongParentType].ToHex(),
		WeakParents:         references[model.WeakParentType].ToHex(),
		ShallowLikeParents:  references[model.ShallowLikeParentType].ToHex(),
		LatestFinalizedSlot: deps.Protocol.MainEngineInstance().Storage.Settings().LatestFinalizedSlot(),
		Commitment:          &commitmentJSONRaw,
	}

	return resp, nil
}

func sendBlock(c echo.Context) (*submitBlockResponse, error) {
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
		case errors.Is(err, blockfactory.ErrBlockAttacherInvalidBlock):
			return nil, errors.WithMessagef(httpserver.ErrInvalidParameter, "failed to attach block: %s", err.Error())

		case errors.Is(err, blockfactory.ErrBlockAttacherAttachingNotPossible):
			return nil, errors.WithMessagef(echo.ErrInternalServerError, "failed to attach block: %s", err.Error())

		case errors.Is(err, blockfactory.ErrBlockAttacherPoWNotAvailable):
			return nil, errors.WithMessagef(echo.ErrServiceUnavailable, "failed to attach block: %s", err.Error())

		default:
			return nil, errors.WithMessagef(echo.ErrInternalServerError, "failed to attach block: %s", err.Error())
		}
	}

	return &submitBlockResponse{
		BlockID: blockID.ToHex(),
	}, nil
}
