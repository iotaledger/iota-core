package v1

import (
	"github.com/iotaledger/hive.go/ds/randommap"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
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
		t.setupBlock(tipBlock)
	}
}

func (t *TipManager) setupBlock(block *Block) {
	t.blockAdded.Trigger(block)

	block.OnTipPoolChanged(func(prevType, newType TipPoolType) {
		if newType == StrongTipPool {
			t.joinStrongTipPool(block)
		}
	})

	block.setTipPool(t.chooseTipPool(block))
}

func (t *TipManager) joinStrongTipPool(block *Block) {
	var unsubscribeEvents func()

	leaveStrongTipPool := func() {
		unsubscribeEvents()

		t.tips.Delete(block.ID())

		// TODO: decrease parent approval count
		t.decreaseParentApprovalCount(block)
	}

	t.increaseParentApprovalCount(block)

	unsubscribeEvents = lo.Batch(
		block.OnStrongApprovalCountUpdated(func(prevValue, newValue int) {
			if prevValue == 0 && newValue == 1 {
				t.tips.Delete(block.ID())
			} else if newValue == 0 {
				t.tips.Set(block.ID(), block) // TODO: reclassify before adding back in
			}
		}),

		block.OnTipPoolChanged(func(prevType, newType TipPoolType) {
			if newType != StrongTipPool {
				leaveStrongTipPool()
			}
		}),
	)
}

func (t *TipManager) chooseTipPool(block *Block, optMinType ...TipPoolType) TipPoolType {
	blockIsVotingForNonRejectedBranches := func(block *Block) bool {
		return true
	}

	payloadIsNotRejected := func(block *Block) bool {
		return true
	}

	switch {
	case lo.First(optMinType) <= StrongTipPool && blockIsVotingForNonRejectedBranches(block):
		return StrongTipPool
	case lo.First(optMinType) <= WeakTipPool && payloadIsNotRejected(block):
		return WeakTipPool
	default:
		return DroppedTipPool
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

func (t *TipManager) decreaseParentApprovalCount(block *Block) {

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
				t.setupBlock(parentBlock)

				t.blockAdded.Trigger(parentBlock)
			}
		}
	})
}
