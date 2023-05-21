package v1

import (
	"github.com/iotaledger/hive.go/ds/randommap"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/iota-core/pkg/core/promise"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	iotago "github.com/iotaledger/iota.go/v4"
)

type TipManager struct {
	retrieveBlock func(blockID iotago.BlockID) (block *blocks.Block, exists bool)
	blocks        *shrinkingmap.ShrinkingMap[iotago.BlockID, *Block]
	strongTips    *randommap.RandomMap[iotago.BlockID, *Block]
	weakTips      *randommap.RandomMap[iotago.BlockID, *Block]
	blockAdded    *event.Event1[*Block]
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
	block.stronglyConnectedToTips.OnUpdate(func(_, isConnected bool) {
		if isConnected {
			t.updateParents(block, model.StrongParentType, (*Block).increaseStronglyConnectedChildren)
			t.updateParents(block, model.WeakParentType, (*Block).increaseWeaklyConnectedChildren)
		} else {
			t.updateParents(block, model.StrongParentType, (*Block).decreaseStronglyConnectedChildren)
			t.updateParents(block, model.WeakParentType, (*Block).decreaseWeaklyConnectedChildren)
		}
	})

	block.weaklyConnectedToTips.OnUpdate(func(_, isConnected bool) {
		if isConnected {
			t.updateParents(block, model.WeakParentType, (*Block).increaseWeaklyConnectedChildren)
		} else {
			t.updateParents(block, model.WeakParentType, (*Block).decreaseWeaklyConnectedChildren)
		}
	})

	joinTipPool := func(tipSet *randommap.RandomMap[iotago.BlockID, *Block], blockReferencedByTips *promise.Value[bool]) (leaveTipPool func()) {
		unsubscribe := blockReferencedByTips.OnUpdate(func(_, isReferenced bool) {
			if isReferenced {
				tipSet.Delete(block.ID())
			} else {
				tipSet.Set(block.ID(), block)
			}
		})

		return func() {
			unsubscribe()

			tipSet.Delete(block.ID())
		}
	}

	var leaveTipPool func()
	block.tipPool.OnUpdate(func(prevTipPool, newTipPool TipPoolType) {
		if leaveTipPool != nil {
			leaveTipPool()
		}

		if newTipPool == StrongTipPool {
			leaveTipPool = joinTipPool(t.strongTips, block.stronglyReferencedByTips)
		} else if newTipPool == WeakTipPool {
			leaveTipPool = joinTipPool(t.weakTips, block.referencedByTips)
		} else {
			leaveTipPool = nil
		}
	})

	block.setTipPool(t.determineInitialTipPool(block))

	t.blockAdded.Trigger(block)
}

func (t *TipManager) determineInitialTipPool(block *Block, optMinType ...TipPoolType) TipPoolType {
	blockIsVotingForNonRejectedBranches := func(block *Block) bool {
		return true
	}

	payloadIsLiked := func(block *Block) bool {
		return true
	}

	if lo.First(optMinType) <= StrongTipPool && blockIsVotingForNonRejectedBranches(block) {
		return StrongTipPool
	}

	if lo.First(optMinType) <= WeakTipPool && payloadIsLiked(block) {
		return WeakTipPool
	}

	return DroppedTipPool
}

func (t *TipManager) updateParents(block *Block, parentType model.ParentsType, updateFunc func(*Block)) {
	block.ForEachParent(func(parent model.Parent) {
		// TODO: MAKE GetOrCreate ignore nil return values
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
