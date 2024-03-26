package requesthandler

import (
	"time"

	"github.com/labstack/echo/v4"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/iota-core/pkg/model"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

func (r *RequestHandler) ModelBlockFromBlockID(blockID iotago.BlockID) (*model.Block, error) {
	block, exists := r.protocol.Engines.Main.Get().Block(blockID)
	if !exists {
		return nil, ierrors.WithMessagef(echo.ErrNotFound, "block %s not found", blockID)
	}

	return block, nil
}

func (r *RequestHandler) BlockFromBlockID(blockID iotago.BlockID) (*iotago.Block, error) {
	block, err := r.ModelBlockFromBlockID(blockID)
	if err != nil {
		return nil, err
	}

	return block.ProtocolBlock(), nil
}

func (r *RequestHandler) BlockMetadataFromBlockID(blockID iotago.BlockID) (*api.BlockMetadataResponse, error) {
	blockMetadata, err := r.protocol.Engines.Main.Get().BlockRetainer.BlockMetadata(blockID)
	if err != nil {
		if ierrors.Is(err, kvstore.ErrKeyNotFound) {
			return nil, ierrors.WithMessagef(echo.ErrNotFound, "block not found: %s: %w", blockID.ToHex(), err)
		}

		return nil, ierrors.WithMessagef(echo.ErrInternalServerError, "failed to get block metadata %s: %w", blockID, err)
	}

	return blockMetadata, nil
}

func (r *RequestHandler) BlockWithMetadataFromBlockID(blockID iotago.BlockID) (*api.BlockWithMetadataResponse, error) {
	block, exists := r.protocol.Engines.Main.Get().Block(blockID)
	if !exists {
		return nil, ierrors.WithMessagef(echo.ErrNotFound, "no transaction found for block ID %s", blockID)
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
		return nil, ierrors.WithMessage(echo.ErrServiceUnavailable, "no strong parents available")
	}

	// get the latest parent block issuing time
	var latestParentBlockIssuingTime time.Time

	checkParent := func(parentBlockID iotago.BlockID) error {
		parentBlock, exists := r.protocol.Engines.Main.Get().Block(parentBlockID)
		if !exists {
			// check if this is the genesis block
			if parentBlockID == r.CommittedAPI().ProtocolParameters().GenesisBlockID() {
				return nil
			}

			// or a root block
			rootBlocks, err := r.protocol.Engines.Main.Get().Storage.RootBlocks(parentBlockID.Slot())
			if err != nil {
				return ierrors.WithMessagef(echo.ErrInternalServerError, "failed to get root blocks for slot %d: %s", parentBlockID.Slot(), err)
			}

			isRootBlock, err := rootBlocks.Has(parentBlockID)
			if err != nil {
				return ierrors.WithMessagef(echo.ErrInternalServerError, "failed to check if block %s is a root block: %w", parentBlockID, err)
			}

			if isRootBlock {
				return nil
			}

			return ierrors.WithMessagef(echo.ErrNotFound, "no block found for block ID %s", parentBlockID)
		}

		if latestParentBlockIssuingTime.Before(parentBlock.ProtocolBlock().Header.IssuingTime) {
			latestParentBlockIssuingTime = parentBlock.ProtocolBlock().Header.IssuingTime
		}

		return nil
	}

	for _, parentType := range []iotago.ParentsType{iotago.StrongParentType, iotago.WeakParentType, iotago.ShallowLikeParentType} {
		for _, parentBlockID := range references[parentType] {
			if err := checkParent(parentBlockID); err != nil {
				return nil, ierrors.Wrap(err, "failed to retrieve parents")
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
