package core

import (
	"github.com/labstack/echo/v4"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/inx-app/pkg/httpserver"
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
	slotCommitment := deps.Protocol.MainEngineInstance().SyncManager.LatestCommitment()

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
