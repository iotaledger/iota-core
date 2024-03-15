package requesthandler

import (
	"time"

	"github.com/labstack/echo/v4"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/inx-app/pkg/httpserver"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

func (r *RequestHandler) BlockByID(blockID iotago.BlockID) (*iotago.Block, error) {
	block, exists := r.protocol.Engines.Main.Get().Block(blockID)
	if !exists {
		return nil, ierrors.WithMessagef(echo.ErrNotFound, "block %s not found", blockID)
	}

	return block.ProtocolBlock(), nil
}

func (r *RequestHandler) BlockMetadataByBlockID(blockID iotago.BlockID) (*api.BlockMetadataResponse, error) {
	blockMetadata, err := r.protocol.Engines.Main.Get().BlockRetainer.BlockMetadata(blockID)
	if err != nil {
		if ierrors.Is(err, kvstore.ErrKeyNotFound) {
			return nil, ierrors.WithMessagef(echo.ErrNotFound, "block %s not found", blockID)
		}

		return nil, ierrors.WithMessagef(echo.ErrInternalServerError, "failed to get block metadata %s: %w", blockID, err)
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
		return nil, ierrors.WithMessagef(echo.ErrNotFound, "no transaction found for block ID %s", blockID)
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
		return nil, ierrors.WithMessage(echo.ErrServiceUnavailable, "no strong parents available")
	}

	// get the latest parent block issuing time
	var latestParentBlockIssuingTime time.Time
	for _, parentType := range []iotago.ParentsType{iotago.StrongParentType, iotago.WeakParentType, iotago.ShallowLikeParentType} {
		for _, blockID := range references[parentType] {
			block, exists := r.protocol.Engines.Main.Get().Block(blockID)
			if !exists {
				return nil, ierrors.WithMessagef(echo.ErrNotFound, "failed to retrieve parents: no block found for block ID %s", blockID)
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
