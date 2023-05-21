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
	blocks     *shrinkingmap.ShrinkingMap[iotago.BlockID, *Block]
	strongTips *randommap.RandomMap[iotago.BlockID, *Block]
	weakTips   *randommap.RandomMap[iotago.BlockID, *Block]

	blockAdded *event.Event1[*Block]

	retrieveBlock func(blockID iotago.BlockID) (block *blocks.Block, exists bool)
}

func NewTipManager(retrieveBlock func(blockID iotago.BlockID) (block *blocks.Block, exists bool)) *TipManager {
	return &TipManager{
		blocks:        shrinkingmap.New[iotago.BlockID, *Block](),
		strongTips:    randommap.New[iotago.BlockID, *Block](),
		blockAdded:    event.New1[*Block](),
		retrieveBlock: retrieveBlock,
	}
}

func (t *TipManager) AddBlock(block *blocks.Block) {
	if tipBlock := NewBlock(block); t.blocks.Set(block.ID(), tipBlock) {
		t.setupBlock(tipBlock)
	}
}

func (t *TipManager) RemoveBlock(blockID iotago.BlockID) {
	if tipBlock, removed := t.blocks.DeleteAndReturn(blockID); removed {
		tipBlock.blockEvicted.Trigger()
	}
}

func (t *TipManager) OnBlockAdded(handler func(block *Block)) (unsubscribe func()) {
	return t.blockAdded.Hook(handler).Unhook
}

func (t *TipManager) setupBlock(block *Block) {
	block.stronglyReachableFromTips.OnUpdate(func(_, reachable bool) {
		if reachable {
			t.updateParents(block, model.StrongParentType, func(parentBlock *Block) { parentBlock.stronglyConnectedChildren.Increase() })
			t.updateParents(block, model.WeakParentType, func(parentBlock *Block) { parentBlock.weaklyConnectedChildren.Increase() })
		} else {
			t.updateParents(block, model.StrongParentType, func(parentBlock *Block) { parentBlock.stronglyConnectedChildren.Decrease() })
			t.updateParents(block, model.WeakParentType, func(parentBlock *Block) { parentBlock.weaklyConnectedChildren.Decrease() })
		}
	})

	block.weaklyReachableFromTips.OnUpdate(func(_, reachable bool) {
		if reachable {
			t.updateParents(block, model.WeakParentType, func(parentBlock *Block) { parentBlock.weaklyConnectedChildren.Increase() })
		} else {
			t.updateParents(block, model.WeakParentType, func(parentBlock *Block) { parentBlock.weaklyConnectedChildren.Decrease() })
		}
	})

	setupTipPoolEvents := func(tipPool *randommap.RandomMap[iotago.BlockID, *Block], onConnectedChildrenUpdated func(func(int, int)) func()) func() {
		return onConnectedChildrenUpdated(func(from, to int) {
			if from == 0 && to == 1 {
				tipPool.Delete(block.ID())
			} else if to == 0 {
				tipPool.Set(block.ID(), block)
			}
		})
	}

	var unhookTipPoolEvents func()

	block.tipPool.OnUpdate(func(prevType, newType TipPoolType) {
		if unhookTipPoolEvents != nil {
			unhookTipPoolEvents()

			switch prevType {
			case StrongTipPool:
				t.strongTips.Delete(block.ID())
			case WeakTipPool:
				t.weakTips.Delete(block.ID())
			}
		}

		switch newType {
		case StrongTipPool:
			unhookTipPoolEvents = setupTipPoolEvents(t.strongTips, block.OnStronglyConnectedChildrenUpdated)
		case WeakTipPool:
			unhookTipPoolEvents = setupTipPoolEvents(t.weakTips, block.OnWeaklyConnectedChildrenUpdated)
		default:
			unhookTipPoolEvents = nil
		}
	})

	block.setTipPool(t.determineTipPool(block))

	t.blockAdded.Trigger(block)
}

func (t *TipManager) determineTipPool(block *Block, optMinType ...TipPoolType) TipPoolType {
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

func (t *TipManager) updateParents(block *Block, parentType model.ParentsType, updateFunc func(*Block)) {
	block.ForEachParent(func(parent model.Parent) {
		if parentBlock, created := t.blocks.GetOrCreate(parent.ID, func() *Block {
			parentBlock, parentBlockExists := t.retrieveBlock(parent.ID)
			if !parentBlockExists {
				return nil
			}

			return NewBlock(parentBlock)
		}); parentBlock != nil && parent.Type == parentType {
			updateFunc(parentBlock)

			if created {
				t.setupBlock(parentBlock)
			}
		}
	})
}
