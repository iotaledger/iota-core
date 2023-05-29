package bic

import (
	"fmt"
	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/storage/prunable"
	"sync"

	"github.com/iotaledger/hive.go/ads"
	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	iotago "github.com/iotaledger/iota.go/v4"
)

// BICManager is a Block Issuer Credits module responsible for tracking block issuance credit balances.
type BICManager struct {
	workers *workerpool.Group

	// slot diffs for the BIC between [LatestCommitedSlot - MCA, LatestCommitedSlot]
	slotDiffFunc func(iotago.SlotIndex) *prunable.BicDiffs
	// todo add in memory shrink version of the slot diffs

	blockCache *blocks.Blocks
	blockBurns *shrinkingmap.ShrinkingMap[iotago.SlotIndex, *advancedset.AdvancedSet[iotago.BlockID]]

	// TODO: store the index
	// the slot index of the bic vector ("LatestCommitedSlot")
	bicIndex iotago.SlotIndex

	// todo abstact BICTree into separate component to hide the diff moving implementation
	// TODO on reading from the snapshot: create the BIC tree from the bic vector and the slot diffs
	// bic represents the Block Issuer Credits vector of all registered accounts for bicTreeindex slot, it is updated on the slot commitment.
	bicTree *ads.Map[iotago.AccountID, accounts.AccountImpl, *iotago.AccountID, *accounts.AccountImpl]

	apiProviderFunc func() iotago.API
	mutex           sync.RWMutex

	module.Module
}

func (b *BICManager) trackBlock(block *blocks.Block) {
	set, _ := b.blockBurns.GetOrCreate(block.ID().Index(), func() *advancedset.AdvancedSet[iotago.BlockID] {
		return advancedset.New[iotago.BlockID]()
	})
	set.Add(block.ID())
}

func NewProvider(opts ...options.Option[BICManager]) module.Provider[*engine.Engine, accounts.BlockIssuanceCredits] {
	return module.Provide(func(e *engine.Engine) accounts.BlockIssuanceCredits {

		return options.Apply(&BICManager{
			blockBurns: shrinkingmap.New[iotago.SlotIndex, *advancedset.AdvancedSet[iotago.BlockID]](),
			bicTree:    ads.NewMap[iotago.AccountID, accounts.AccountImpl](e.Storage.Accounts()),
			blockCache: e.BlockCache,
			workers:    e.Workers.CreateGroup("BICManager"),
		}, opts, func(b *BICManager) {
			e.HookConstructed(func() {
				wpBic := b.workers.CreatePool("trackBurnt", 1)
				e.Events.BlockGadget.BlockRatifiedAccepted.Hook(b.trackBlock, event.WithWorkerPool(wpBic))
			})
		})
	})
}

func (b *BICManager) API() iotago.API {
	return b.apiProviderFunc()
}

func (b *BICManager) BICTreeRoot() iotago.Identifier {
	return iotago.Identifier(b.bicTree.Root())
}

func (b *BICManager) CommitSlot(slotIndex iotago.SlotIndex, allotments map[iotago.AccountID]uint64) (bicRoot iotago.Identifier, err error) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	if b.bicIndex+1 != slotIndex {
		return iotago.Identifier{}, errors.Errorf("cannot apply the ned diff, there is a gap in committed slots, bic vector index: %d, slot to commit: %d", b.bicIndex, slotIndex)
	}

	burns := make(map[iotago.AccountID]uint64)
	if set, exists := b.blockBurns.Get(slotIndex); exists {
		for it := set.Iterator(); it.HasNext(); {
			blockID := it.Next()
			block, exists := b.blockCache.Block(blockID)
			if !exists {
				return iotago.Identifier{}, errors.Errorf("cannot apply the new diff, block %s not found in the block cache", blockID)
			}
			burns[block.Block().IssuerID] += block.Block().BurnedMana
		}
	}

	changes := newSummarisedChanges(allotments, burns)

	if bicRoot, err = b.applyDiff(slotIndex, changes); err != nil {
		return iotago.Identifier{}, err
	}

	// at this point the bic vector is at bicIndex == slotIndex
	b.removeOldDiff(slotIndex - iotago.MaxCommitableSlotAge - 1)

	return bicRoot, nil
}

func (b *BICManager) BIC(accountID iotago.AccountID, slotIndex iotago.SlotIndex) (accounts.Account, error) {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	if slotIndex < b.bicIndex-iotago.MaxCommitableSlotAge {
		return nil, fmt.Errorf("can't calculate BIC, slot index older than bicIndex (%d<%d)", slotIndex, b.bicIndex)
	}
	if slotIndex > b.bicIndex {
		return nil, fmt.Errorf("can't retrieve BIC, slot %d is not committed yet, lastest committed slot: %d", slotIndex, b.bicIndex)
	}
	// read start value from the bic vector at b.bicIndex
	loadedAccount, exists := b.bicTree.Get(accountID)
	if !exists {
		loadedAccount = accounts.NewAccount(b.API(), accountID, accounts.NewCredits(0, slotIndex))
	}
	// last applied diff should be the diff for slotIndex + 1
	for diffIndex := b.bicIndex; diffIndex > slotIndex; diffIndex-- {
		diffStore := b.slotDiffFunc(diffIndex)
		if diffStore == nil {
			return nil, fmt.Errorf("can't retrieve BIC, could not find diff store for slot (%d)", diffIndex)
		}
		change, err := diffStore.Load(accountID)
		if err != nil {
			return nil, fmt.Errorf("can't calculate BIC, slot index doesn't exist (%d)", diffIndex)
		}
		loadedAccount.Credits().Update(-change)
	}
	return loadedAccount, nil

}

func (b *BICManager) Shutdown() {
	// TODO implement shutdown
}

func (b *BICManager) applyDiff(slotIndex iotago.SlotIndex, changes map[iotago.AccountID]int64) (iotago.Identifier, error) {
	diffStore := b.slotDiffFunc(slotIndex)
	for accountID, valueChange := range changes {
		err := diffStore.Store(accountID, valueChange)
		if err != nil {
			return iotago.Identifier{}, errors.Wrapf(err, "could not store diff to slot %d", slotIndex)
		}
	}

	bicRoot, err := b.commitBICTree(slotIndex, changes)
	if err != nil {
		return iotago.Identifier{}, errors.Wrapf(err, "could not apply diff to slot %d", slotIndex)
	}
	// set the new balances index
	b.bicIndex = slotIndex

	return bicRoot, nil
}

func (b *BICManager) commitBICTree(index iotago.SlotIndex, changes map[iotago.AccountID]int64) (bicRoot iotago.Identifier, err error) {
	// update the bic tree to latestCommitted slot index
	for accountID, valueChange := range changes {
		loadedAccount, exists := b.bicTree.Get(accountID)
		if !exists {
			loadedAccount = accounts.NewAccount(b.API(), accountID, accounts.NewCredits(0, index), nil)
		}
		loadedAccount.Credits().Update(valueChange, index)
		b.bicTree.Set(accountID, loadedAccount)
	}
	b.bicIndex = index

	return b.BICTreeRoot(), nil
}

func (b *BICManager) removeOldDiff(index iotago.SlotIndex) {
	// todo delete the old slot diff that was applied

}

// newBICDiff creates a new BIC diff from the given allotments and burns.
func newSummarisedChanges(allotments map[iotago.AccountID]uint64, burns map[iotago.AccountID]uint64) map[iotago.AccountID]int64 {
	bicChanges := make(map[iotago.AccountID]int64)
	for id, allotment := range allotments {
		bicChanges[id] += int64(allotment)
	}
	for id, burn := range burns {
		bicChanges[id] -= int64(burn)
	}

	return bicChanges
}
