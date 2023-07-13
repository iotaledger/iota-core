package debugapi

import (
	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore/mapdb"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/nodeclient"
)

func getSlotBlockIDs(index iotago.SlotIndex) (*nodeclient.BlockChangesResponse, error) {
	blocksForSlot := deps.Protocol.MainEngineInstance().Storage.Blocks(index)
	if blocksForSlot == nil {
		return nil, ierrors.Errorf("cannot find block storage bucket for slot %d", index)
	}

	includedBlocks := make([]string, 0)
	tangleTree := ads.NewSet(mapdb.NewMapDB(), iotago.SlotIdentifier.Bytes, iotago.SlotIdentifierFromBytes)

	blocksForSlot.ForEachBlockIDInSlot(func(blockID iotago.BlockID) error {
		includedBlocks = append(includedBlocks, blockID.String())
		tangleTree.Add(blockID)
		return nil
	})

	return &nodeclient.BlockChangesResponse{
		Index:          index,
		IncludedBlocks: includedBlocks,
		TangleRoot:     iotago.Identifier(tangleTree.Root()).String(),
	}, nil
}
