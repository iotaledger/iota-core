package requesthandler

import (
	"time"

	"github.com/labstack/echo/v4"

	"github.com/iotaledger/hive.go/ierrors"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

func (r *RequestHandler) BlockFromBlockID(blockID iotago.BlockID) (*iotago.Block, error) {
	block, exists := r.protocol.Engines.Main.Get().Block(blockID)
	if !exists {
		return nil, ierrors.Wrapf(echo.ErrNotFound, "block not found: %s", blockID.ToHex())
	}

	return block.ProtocolBlock(), nil
}

func (r *RequestHandler) BlockMetadataFromBlockID(blockID iotago.BlockID) (*api.BlockMetadataResponse, error) {
	blockMetadata, err := r.protocol.Engines.Main.Get().Retainer.BlockMetadata(blockID)
	if err != nil {
		return nil, ierrors.Wrapf(echo.ErrInternalServerError, "failed to get block metadata %s: %s", blockID.ToHex(), err)
	}

	return blockMetadata.BlockMetadataResponse(), nil
}

func (r *RequestHandler) TransactionMetadataFromBlockID(blockID iotago.BlockID) (*api.TransactionMetadataResponse, error) {
	blockMetadata, err := r.protocol.Engines.Main.Get().Retainer.BlockMetadata(blockID)
	if err != nil {
		return nil, ierrors.Wrapf(echo.ErrInternalServerError, "failed to get block metadata %s: %s", blockID.ToHex(), err)
	}

	metadata := blockMetadata.TransactionMetadataResponse()
	if metadata == nil {
		return nil, ierrors.Wrapf(echo.ErrNotFound, "transaction not found")
	}

	return metadata, nil
}

func (r *RequestHandler) BlockWithMetadataFromBlockID(blockID iotago.BlockID) (*api.BlockWithMetadataResponse, error) {
	block, exists := r.protocol.Engines.Main.Get().Block(blockID)
	if !exists {
		return nil, ierrors.Wrapf(echo.ErrNotFound, "no transaction found for block ID %s", blockID.ToHex())
	}

	blockMetadata, err := r.BlockMetadataFromBlockID(blockID)
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
