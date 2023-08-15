package core

import (
	"io"

	"github.com/labstack/echo/v4"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/inx-app/pkg/httpserver"
	"github.com/iotaledger/iota-core/pkg/blockfactory"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/restapi"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/nodeclient/apimodels"
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

func blockMetadataByBlockID(blockID iotago.BlockID) (*apimodels.BlockMetadataResponse, error) {
	blockMetadata, err := deps.Protocol.MainEngineInstance().Retainer.BlockMetadata(blockID)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to get block metadata: %s", blockID.ToHex())
	}

	return blockMetadata.BlockMetadataResponse(), nil
}

func blockMetadataByID(c echo.Context) (*apimodels.BlockMetadataResponse, error) {
	blockID, err := httpserver.ParseBlockIDParam(c, restapi.ParameterBlockID)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to parse block ID: %s", c.Param(restapi.ParameterBlockID))
	}

	return blockMetadataByBlockID(blockID)
}

func blockIssuance(_ echo.Context) (*apimodels.IssuanceBlockHeaderResponse, error) {
	references := deps.Protocol.MainEngineInstance().TipSelection.SelectTips(iotago.BlockMaxParents)
	slotCommitment := deps.Protocol.SyncManager.LatestCommitment()

	if len(references[iotago.StrongParentType]) == 0 {
		return nil, ierrors.Wrap(echo.ErrServiceUnavailable, "get references failed")
	}

	resp := &apimodels.IssuanceBlockHeaderResponse{
		StrongParents:       references[iotago.StrongParentType],
		WeakParents:         references[iotago.WeakParentType],
		ShallowLikeParents:  references[iotago.ShallowLikeParentType],
		LatestFinalizedSlot: deps.Protocol.SyncManager.LatestFinalizedSlot(),
		Commitment:          slotCommitment.Commitment(),
	}

	return resp, nil
}

func sendBlock(c echo.Context) (*apimodels.BlockCreatedResponse, error) {
	mimeType, err := httpserver.GetRequestContentType(c, httpserver.MIMEApplicationVendorIOTASerializerV2, echo.MIMEApplicationJSON)
	if err != nil {
		return nil, ierrors.Wrapf(httpserver.ErrInvalidParameter, "invalid block, error: %w", err)
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

	case httpserver.MIMEApplicationVendorIOTASerializerV2:
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

	blockID, err := deps.BlockIssuer.AttachBlock(c.Request().Context(), iotaBlock)
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

	return &apimodels.BlockCreatedResponse{
		BlockID: blockID,
	}, nil
}
