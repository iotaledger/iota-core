package debugapi

import (
	"sort"

	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore/mapdb"
	iotago "github.com/iotaledger/iota.go/v4"
)

func getSlotBlockIDs(index iotago.SlotIndex) (*BlockChangesResponse, error) {
	blocksForSlot, err := deps.Protocol.MainEngine.Get().Storage.Blocks(index)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to get block storage bucket for slot %d", index)
	}

	includedBlocks := make([]string, 0)
	tangleTree := ads.NewSet(mapdb.NewMapDB(), iotago.BlockID.Bytes, iotago.BlockIDFromBytes)

	_ = blocksForSlot.StreamKeys(func(blockID iotago.BlockID) error {
		includedBlocks = append(includedBlocks, blockID.String())
		if err := tangleTree.Add(blockID); err != nil {
			return ierrors.Wrapf(err, "failed to add block to tangle tree, blockID: %s", blockID.ToHex())
		}

		return nil
	})

	sort.Strings(includedBlocks)

	return &BlockChangesResponse{
		Index:          index,
		IncludedBlocks: includedBlocks,
		TangleRoot:     iotago.Identifier(tangleTree.Root()).String(),
	}, nil
}
