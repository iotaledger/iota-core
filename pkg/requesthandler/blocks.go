package requesthandler

import (
	"time"

	"github.com/labstack/echo/v4"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/inx-app/pkg/httpserver"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

func (r *RequestHandler) BlockByID(blockID iotago.BlockID) (*iotago.Block, error) {
	block, exists := r.protocol.Engines.Main.Get().Block(blockID)
	if !exists {
		return nil, ierrors.Wrapf(echo.ErrNotFound, "block not found: %s", blockID.ToHex())
	}

	return block.ProtocolBlock(), nil
}

func (r *RequestHandler) BlockMetadataByBlockID(blockID iotago.BlockID) (*api.BlockMetadataResponse, error) {
	blockMetadata, err := r.protocol.Engines.Main.Get().BlockRetainer.BlockMetadata(blockID)
	if err != nil {
		return nil, ierrors.Wrapf(echo.ErrInternalServerError, "failed to get block metadata %s: %s", blockID.ToHex(), err)
	}

	return blockMetadata, nil
}

func (r *RequestHandler) BlockMetadataByID(c echo.Context) (*api.BlockMetadataResponse, error) {
	blockID, err := httpserver.ParseBlockIDParam(c, api.ParameterBlockID)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to parse block ID %s", c.Param(api.ParameterBlockID))
	}

	return r.BlockMetadataByBlockID(blockID)
}

func (r *RequestHandler) BlockWithMetadataByID(blockID iotago.BlockID) (*api.BlockWithMetadataResponse, error) {
	block, exists := r.protocol.Engines.Main.Get().Block(blockID)
	if !exists {
		return nil, ierrors.Wrapf(echo.ErrNotFound, "no transaction found for block ID %s", blockID.ToHex())
	}

	blockMetadata, err := r.BlockMetadataByBlockID(blockID)
	if err != nil {
		return nil, err
	}

	return &api.BlockWithMetadataResponse{
		Block:    block.ProtocolBlock(),
		Metadata: blockMetadata,
	}, nil
}

func (r *RequestHandler) BlockIssuance() (*api.IssuanceBlockHeaderResponse, error) {
	references := r.protocol.Engines.Main.Get().TipSelection.SelectTips(iotago.BasicBlockMaxParents)
	if len(references[iotago.StrongParentType]) == 0 {
		return nil, ierrors.Wrap(echo.ErrServiceUnavailable, "no strong parents available")
	}

	// get the latest parent block issuing time
	var latestParentBlockIssuingTime time.Time
	for _, parentType := range []iotago.ParentsType{iotago.StrongParentType, iotago.WeakParentType, iotago.ShallowLikeParentType} {
		for _, blockID := range references[parentType] {
			block, exists := r.protocol.Engines.Main.Get().Block(blockID)
			if !exists {
				return nil, ierrors.Wrapf(echo.ErrNotFound, "no block found for parent, block ID: %s", blockID.ToHex())
			}

			if latestParentBlockIssuingTime.Before(block.ProtocolBlock().Header.IssuingTime) {
				latestParentBlockIssuingTime = block.ProtocolBlock().Header.IssuingTime
			}
		}
	}

	resp := &api.IssuanceBlockHeaderResponse{
		StrongParents:                references[iotago.StrongParentType],
		WeakParents:                  references[iotago.WeakParentType],
		ShallowLikeParents:           references[iotago.ShallowLikeParentType],
		LatestParentBlockIssuingTime: latestParentBlockIssuingTime,
		LatestFinalizedSlot:          r.protocol.Engines.Main.Get().SyncManager.LatestFinalizedSlot(),
		LatestCommitment:             r.protocol.Engines.Main.Get().SyncManager.LatestCommitment().Commitment(),
	}

	return resp, nil
}
