package coreapi

import (
	"encoding/json"
	"io"

	"github.com/labstack/echo/v4"

	"github.com/iotaledger/hive.go/ierrors"
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
		return nil, ierrors.Wrapf(err, "failed to parse block ID: %s", c.Param(restapi.ParameterBlockID))
	}

	block, exists := deps.Protocol.MainEngineInstance().Block(blockID)
	if !exists {
		return nil, ierrors.Errorf("block not found: %s", blockID.ToHex())
	}

	return block, nil
}

func blockMetadataResponseByID(c echo.Context) (*nodeclient.BlockMetadataResponse, error) {
	block, err := blockByID(c)
	if err != nil {
		return nil, err
	}

	txState := txStatePending.String()
	txMetadata, exist := deps.Protocol.MainEngineInstance().Ledger.TransactionMetadataByAttachment(block.ID())
	if exist {
		txState = resolveTxState(txMetadata)
	}

	// TODO: fill in blockReason, TxState, TxReason.
	bmResponse := &nodeclient.BlockMetadataResponse{
		BlockID:            block.ID().ToHex(),
		StrongParents:      block.ProtocolBlock().Block.StrongParentIDs().ToHex(),
		WeakParents:        block.ProtocolBlock().Block.WeakParentIDs().ToHex(),
		ShallowLikeParents: block.ProtocolBlock().Block.ShallowLikeParentIDs().ToHex(),
		TxState:            txState,
		BlockState:         blockStatePending.String(),
	}

	return bmResponse, nil
}

func blockIssuance(_ echo.Context) (*nodeclient.BlockIssuanceResponse, error) {
	references := deps.Protocol.MainEngineInstance().TipSelection.SelectTips(iotago.BlockMaxParents)
	slotCommitment := deps.Protocol.SyncManager.LatestCommitment()

	if len(references[iotago.StrongParentType]) == 0 {
		return nil, ierrors.Wrap(echo.ErrServiceUnavailable, "get references failed")
	}

	cBytes, err := deps.Protocol.APIForSlot(slotCommitment.Index()).JSONEncode(slotCommitment.Commitment())
	if err != nil {
		return nil, err
	}
	commitmentJSONRaw := json.RawMessage(cBytes)

	resp := &nodeclient.BlockIssuanceResponse{
		StrongParents:       references[iotago.StrongParentType].ToHex(),
		WeakParents:         references[iotago.WeakParentType].ToHex(),
		ShallowLikeParents:  references[iotago.ShallowLikeParentType].ToHex(),
		LatestFinalizedSlot: deps.Protocol.SyncManager.LatestFinalizedSlot(),
		Commitment:          &commitmentJSONRaw,
	}

	return resp, nil
}

func sendBlock(c echo.Context) (*submitBlockResponse, error) {
	mimeType, err := httpserver.GetRequestContentType(c, httpserver.MIMEApplicationVendorIOTASerializerV1, echo.MIMEApplicationJSON)
	if err != nil {
		return nil, err
	}

	var iotaBlock = &iotago.ProtocolBlock{}

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
		if err := deps.Protocol.CurrentAPI().JSONDecode(bytes, iotaBlock); err != nil {
			return nil, ierrors.Wrapf(httpserver.ErrInvalidParameter, "invalid block, error: %w", err)
		}

	case httpserver.MIMEApplicationVendorIOTASerializerV1:
		version, _, err := iotago.VersionFromBytes(bytes)
		if err != nil {
			return nil, ierrors.Wrapf(httpserver.ErrInvalidParameter, "invalid block, error: %w", err)
		}

		apiForVersion, err := deps.Protocol.APIForVersion(version)
		if err != nil {
			return nil, ierrors.Wrapf(httpserver.ErrInvalidParameter, "invalid block, error: %w", err)
		}

		// Do not validate here, the parents might need to be set
		if _, err := apiForVersion.Decode(bytes, iotaBlock); err != nil {
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

	return &submitBlockResponse{
		BlockID: blockID.ToHex(),
	}, nil
}
