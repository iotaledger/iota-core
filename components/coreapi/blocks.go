package coreapi

import (
	"io"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/inx-app/pkg/httpserver"
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

	bmResponse := &blockMetadataResponse{
		BlockID:            block.ID().ToHex(),
		StrongParents:      block.Block().StrongParents.ToHex(),
		WeakParents:        block.Block().WeakParents.ToHex(),
		ShallowLikeParents: block.Block().ShallowLikeParents.ToHex(),
		BlockState:         blockStatePending.String(),
	}
	_, exists := deps.Protocol.MainEngineInstance().Block(block.ID())
	if !exists {
		bmResponse.BlockStateReason = "block not found"
	}

	// todo set states and error

	return bmResponse, nil
}

func blockIssuance(_ echo.Context) (*blockIssuanceResponse, error) {
	references := deps.Protocol.TipManager.Tips(iotago.BlockMaxParents)
	parentsMaxTime := time.Time{}
	parents := lo.Flatten(lo.Map[iotago.BlockIDs, []iotago.BlockID](lo.Values(references), func(ds iotago.BlockIDs) []iotago.BlockID { return ds }))
	for _, parent := range parents {
		if b, exists := deps.Protocol.MainEngineInstance().BlockFromCache(parent); exists {
			if b.IssuingTime().After(parentsMaxTime) {
				parentsMaxTime = b.IssuingTime()
			}
		}
	}

	if parentsMaxTime.After(time.Now()) {
		return nil, errors.Errorf("cannot issue block if the parents issuingTime is ahead of our local clock: %s vs %s", parentsMaxTime, time.Now())
	}

	slotCommitment := deps.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment()

	resp := &blockIssuanceResponse{
		StrongParents:       references[model.StrongParentType].ToHex(),
		WeakParents:         references[model.WeakParentType].ToHex(),
		ShallowLikeParents:  references[model.ShallowLikeParentType].ToHex(),
		LatestFinalizedSlot: deps.Protocol.MainEngineInstance().Storage.Settings().LatestFinalizedSlot(),
		Commitment:          slotCommitment.Commitment(),
	}

	return resp, nil
}

func sendBlock(_ echo.Context) (*blockCreatedResponse, error) {
	//nolint:nilnil // temporary nil,nil
	return nil, nil
}
