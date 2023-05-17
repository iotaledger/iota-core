package v1

import (
	"github.com/iotaledger/hive.go/ds/randommap"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	iotago "github.com/iotaledger/iota.go/v4"
)

type TipManager struct {
	trackedBlocks *shrinkingmap.ShrinkingMap[iotago.BlockID, *Block]
	tips          *randommap.RandomMap[iotago.BlockID, *Block]

	blockAdded *event.Event1[*Block]

	retrieveBlock func(blockID iotago.BlockID) (block *blocks.Block, exists bool)
}

func NewTipManager(retrieveBlock func(blockID iotago.BlockID) (block *blocks.Block, exists bool)) *TipManager {
	return &TipManager{
		trackedBlocks: shrinkingmap.New[iotago.BlockID, *Block](),
		tips:          randommap.New[iotago.BlockID, *Block](),
		blockAdded:    event.New1[*Block](),
		retrieveBlock: retrieveBlock,
	}
}

func (t *TipManager) AddBlock(block *blocks.Block) {
	if tipBlock := NewBlock(block); t.trackedBlocks.Set(block.ID(), tipBlock) {
		t.increaseParentApprovalCount(tipBlock)

		t.blockAdded.Trigger(tipBlock)
	}
}

func (t *TipManager) RemoveBlock(blockID iotago.BlockID) {
	if tipBlock, removed := t.trackedBlocks.DeleteAndReturn(blockID); removed {
		tipBlock.blockEvicted.Trigger()
	}
}

func (t *TipManager) OnBlockAdded(handler func(block *Block)) (unsubscribe func()) {
	return t.blockAdded.Hook(handler).Unhook
}

func (t *TipManager) increaseParentApprovalCount(block *Block) {
	block.ForEachParent(func(parent model.Parent) {
		if parentBlock, created := t.trackedBlocks.GetOrCreate(parent.ID, func() *Block {
			if parentBlock, parentBlockExists := t.retrieveBlock(parent.ID); parentBlockExists {
				return NewBlock(parentBlock)
			}

			return nil
		}); parentBlock != nil {
			parentBlock.IncreaseApprovalCount()

			if created {
				if parent.Type == model.StrongParentType {
					t.increaseParentApprovalCount(parentBlock)
				}

				t.blockAdded.Trigger(parentBlock)
			}
		}
	})
}
