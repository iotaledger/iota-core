package trivialtipmanager

import (
	"fmt"
	"sync"
	"time"

	"github.com/iotaledger/hive.go/core/memstorage"
	"github.com/iotaledger/hive.go/ds/randommap"
	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/eviction"
	"github.com/iotaledger/iota-core/pkg/protocol/tipmanager"
	iotago "github.com/iotaledger/iota.go/v4"
)

type (
	blockRetrieverFunc func(id iotago.BlockID) (block *blocks.Block, exists bool)
	isBootstrappedFunc func() bool
)

// region TipManager ///////////////////////////////////////////////////////////////////////////////////////////////////

// TipManager is a trivial implementation of the TipManager interface without acceptance clock, and TSC.
type TipManager struct {
	events *tipmanager.Events

	evictionState *eviction.State
	// blockAcceptanceGadget blockgadget.Gadget

	workers            *workerpool.Group
	blockRetrieverFunc blockRetrieverFunc
	isBootstrappedFunc isBootstrappedFunc

	walkerCache *memstorage.IndexedStorage[iotago.SlotIndex, iotago.BlockID, types.Empty]

	mutex sync.RWMutex
	tips  *randommap.RandomMap[iotago.BlockID, *blocks.Block]
	// TipsConflictTracker *TipsConflictTracker

	optsTimeSinceConfirmationThreshold time.Duration

	module.Module
}

// NewProvider creates a new TipManager provider.
func NewProvider(opts ...options.Option[TipManager]) module.Provider[*engine.Engine, tipmanager.TipManager] {
	return module.Provide(func(e *engine.Engine) tipmanager.TipManager {
		t := New(e.Workers.CreateGroup("TipManager"), e.EvictionState, e.Block, e.IsBootstrapped, opts...)

		e.Events.Booker.BlockBooked.Hook(func(block *blocks.Block) {
			_ = t.AddTip(block)
		}, event.WithWorkerPool(t.workers.CreatePool("AddTip", 2)))

		e.Events.EvictionState.SlotEvicted.Hook(t.evict, event.WithWorkerPool(t.workers.CreatePool("SlotEvicted", 1)))

		t.TriggerInitialized()

		return t
	})
}

// New creates a new TipManager.
func New(workers *workerpool.Group, evictionState *eviction.State, blockRetriever blockRetrieverFunc, isBootstrappedFunc isBootstrappedFunc, opts ...options.Option[TipManager]) (t *TipManager) {
	t = options.Apply(&TipManager{
		events:                             tipmanager.NewEvents(),
		evictionState:                      evictionState,
		workers:                            workers,
		blockRetrieverFunc:                 blockRetriever,
		isBootstrappedFunc:                 isBootstrappedFunc,
		tips:                               randommap.New[iotago.BlockID, *blocks.Block](),
		walkerCache:                        memstorage.NewIndexedStorage[iotago.SlotIndex, iotago.BlockID, types.Empty](),
		optsTimeSinceConfirmationThreshold: time.Minute,
	}, opts,
		(*TipManager).TriggerConstructed,
	)

	return
}

// Events returns the events of the TipManager.
func (t *TipManager) Events() *tipmanager.Events {
	return t.events
}

// AddTip adds a Block to the tip pool.
func (t *TipManager) AddTip(block *blocks.Block) (added bool) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	// Check if any children that are accepted or scheduled and return if true, to guarantee that parents are not added
	// to the tipset after their children.
	if t.checkMonotonicity(block) {
		return
	}

	return t.AddTipNonMonotonic(block)
}

// TODO: we might not need this at all
// AddTipNonMonotonic adds a tip to the TipManager without checking for monotonicity.
func (t *TipManager) AddTipNonMonotonic(block *blocks.Block) (added bool) {
	// TODO: add when we have a way to check if a block is invalid
	// if block.IsSubjectivelyInvalid() {
	// 	return
	// }

	// TODO: add when we have conflict tracking
	// Do not add a tip booked on a reject branch, we won't use it as a tip and it will otherwise remove parent tips.
	// blockConflictIDs := t.engine.Tangle.Booker().BlockConflicts(block.Block)
	// if t.engine.Ledger.MemPool().ConflictDAG().ConfirmationState(blockConflictIDs).IsRejected() {
	// 	return
	// }

	added = t.addTip(block)

	// TODO: add when we have conflict tracking
	// if added {
	// 	t.TipsConflictTracker.AddTip(block, blockConflictIDs)
	// }

	return
}

// RemoveTip removes a tip from the TipManager.
func (t *TipManager) RemoveTip(blockID iotago.BlockID) (removed bool) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	block, exists := t.blockRetrieverFunc(blockID)
	if !exists {
		return false
	}

	return t.removeTip(block)
}

// Tips returns count number of tips, maximum MaxParentsCount.
func (t *TipManager) Tips(count int) (tips iotago.BlockIDs) {
	if count > iotago.BlockMaxParents {
		count = iotago.BlockMinParents
	}
	if count < iotago.BlockMinParents {
		count = iotago.BlockMinParents
	}

	return t.selectTips(count)
}

// AllTips returns a list of all tips that are stored in the TipManger.
func (t *TipManager) AllTips() (allTips []*blocks.Block) {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	allTips = make([]*blocks.Block, 0, t.tips.Size())
	t.tips.ForEach(func(_ iotago.BlockID, value *blocks.Block) bool {
		allTips = append(allTips, value)
		return true
	})

	return
}

// TipCount the amount of tips.
func (t *TipManager) TipCount() int {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	return t.tips.Size()
}

func (t *TipManager) Shutdown() {
	t.workers.Shutdown()
	t.TriggerStopped()
}

func (t *TipManager) addTip(block *blocks.Block) (added bool) {
	if !t.tips.Has(block.ID()) {
		t.tips.Set(block.ID(), block)
		// TODO: add when we have conflict tracking
		// t.tipsConflictTracker.AddTip(block)
		t.events.TipAdded.Trigger(block)

		// a tip loses its tip status if it is referenced by another block
		t.removeStrongParents(block)

		return true
	}

	return false
}

func (t *TipManager) removeTip(block *blocks.Block) (deleted bool) {
	if _, deleted = t.tips.Delete(block.ID()); deleted {
		// TODO: add when we have conflict tracking
		// t.TipsConflictTracker.RemoveTip(block)
		t.events.TipRemoved.Trigger(block)
	}
	return
}

// RemoveStrongParents removes all tips that are strong parents of the given block.
func (t *TipManager) removeStrongParents(block *blocks.Block) {
	for _, strongParentID := range block.Block().StrongParents {
		if strongParentBlock, exists := t.blockRetrieverFunc(strongParentID); exists {
			t.removeTip(strongParentBlock)
		}
	}
}

func (t *TipManager) selectTips(count int) (parents iotago.BlockIDs) {
	t.mutex.Lock() // RemoveTip might get called, so we need a write-lock here
	defer t.mutex.Unlock()

	parents = make(iotago.BlockIDs, 0)
	tips := t.tips.RandomUniqueEntries(count)

	// We obtain up to 8 latest root blocks if there is no valid tip and we submit them to the TSC check as some
	// could be old in case of a slow growing BlockDAG.
	if len(tips) == 0 {
		rootBlocks := t.evictionState.LatestRootBlocks()

		for _, blockID := range rootBlocks {
			if block, exist := t.blockRetrieverFunc(blockID); exist {
				tips = append(tips, block)
			}
		}
		fmt.Println("(time: ", time.Now(), ") selecting root blocks because tip pool empty:", rootBlocks)
	}

	for _, tip := range tips {
		if err := t.isValidTip(tip); err == nil {
			parents = append(parents, tip.ID())
		} else {
			t.removeTip(tip)

			// DEBUG
			fmt.Printf("(time: %s) cannot select tip due to error: %s\n", time.Now(), err)
			if t.tips.Size() == 0 {
				fmt.Println("(time: ", time.Now(), ") >> deleted last TIP because it doesn't pass checks!", tip.ID())
			}
		}
	}

	return parents
}

// checkMonotonicity returns true if the block has any accepted or scheduled child.
func (t *TipManager) checkMonotonicity(block *blocks.Block) (anyScheduledOrAccepted bool) {
	for _, child := range block.Children() {
		if child.IsOrphaned() {
			continue
		}

		// TODO: add when we have acceptance
		// if t.blockAcceptanceGadget.IsBlockAccepted(child.ID()) {
		// 	return true
		// }

		// TODO: add when we have a scheduler
		// if childBlock, exists := t.blockRetrieverFunc(child.ID()); exists {
		// 	if childBlock.IsScheduled() {
		return true
		// 	}
		// }
	}

	return false
}

func (t *TipManager) evict(index iotago.SlotIndex) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	t.walkerCache.Evict(index)
}
