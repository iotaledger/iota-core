package bic

import (
	"fmt"
	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/hive.go/kvstore"
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
	// blockBurns stores in memory only, the block IDs of the blocks that were issued per slot up to LatestCommitedSlot,
	// we need it to calculate mana burns on slot commitment
	blockBurns *shrinkingmap.ShrinkingMap[iotago.SlotIndex, *advancedset.AdvancedSet[iotago.BlockID]]
	// todo add in memory shrink version of the slot diffs

	// latestCommittedSlot is where the BIC tree is kept at.
	latestCommittedSlot iotago.SlotIndex

	// TODO on reading from the snapshot: create the BIC tree from the bic vector and the slot diffs
	// bic represents the Block Issuer Credits vector of all registered accounts for bicTreeindex slot, it is updated on the slot commitment.
	bicTree *ads.Map[iotago.AccountID, accounts.AccountData, *iotago.AccountID, *accounts.AccountData]

	// slot diffs for the BIC between [LatestCommitedSlot - MCA, LatestCommitedSlot].
	slotDiffFunc func(iotago.SlotIndex) *prunable.BicDiffs

	// blockFunc is a function that returns a block from the cache or from the database.
	blockFunc func(id iotago.BlockID) (*blocks.Block, bool)

	apiProviderFunc func() iotago.API
	mutex           sync.RWMutex

	// TODO reevaluate locking
	module.Module
}

func New(blocksCache func(id iotago.BlockID) (*blocks.Block, bool), accountsStore kvstore.KVStore) *BICManager {
	return &BICManager{
		blockBurns: shrinkingmap.New[iotago.SlotIndex, *advancedset.AdvancedSet[iotago.BlockID]](),
		bicTree:    ads.NewMap[iotago.AccountID, accounts.AccountData](accountsStore),
		blockFunc:  blocksCache,
	}
}

func (b *BICManager) SetBICIndex(index iotago.SlotIndex) {
	b.latestCommittedSlot = index
}

func (b *BICManager) TrackBlock(block *blocks.Block) {
	set, _ := b.blockBurns.GetOrCreate(block.ID().Index(), func() *advancedset.AdvancedSet[iotago.BlockID] {
		return advancedset.New[iotago.BlockID]()
	})
	set.Add(block.ID())
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
	if b.latestCommittedSlot+1 != slotIndex {
		return iotago.Identifier{}, errors.Errorf("cannot apply the ned diff, there is a gap in committed slots, bic vector index: %d, slot to commit: %d", b.latestCommittedSlot, slotIndex)
	}

	burns := make(map[iotago.AccountID]uint64)
	if set, exists := b.blockBurns.Get(slotIndex); exists {
		for it := set.Iterator(); it.HasNext(); {
			blockID := it.Next()
			block, exists := b.blockFunc(blockID)
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

	if slotIndex < b.latestCommittedSlot-iotago.MaxCommitableSlotAge {
		return nil, fmt.Errorf("can't calculate BIC, slot index older than bicIndex (%d<%d)", slotIndex, b.latestCommittedSlot)
	}
	if slotIndex > b.latestCommittedSlot {
		return nil, fmt.Errorf("can't retrieve BIC, slot %d is not committed yet, lastest committed slot: %d", slotIndex, b.latestCommittedSlot)
	}
	// read start value from the bic vector at b.bicIndex
	loadedAccount, exists := b.bicTree.Get(accountID)
	if !exists {
		loadedAccount = accounts.NewAccount(b.API(), accountID, accounts.NewCredits(0, slotIndex))
	}
	// last applied diff should be the diff for slotIndex + 1
	for diffIndex := b.latestCommittedSlot; diffIndex > slotIndex; diffIndex-- {
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

// BICDiffTo returns the diff between the BIC at index and current BIC vector at BICIndex.
func (b *BICManager) BICDiffTo(targetSlot iotago.SlotIndex) map[iotago.AccountID]*accounts.Credits {
	changes := make(map[iotago.AccountID]*accounts.Credits)
	for index := targetSlot + 1; index <= b.latestCommittedSlot; index++ {
		diffStore := b.slotDiffFunc(index)
		if diffStore == nil {
			return nil
		}
		diffStore.Stream(func(accountID iotago.AccountID, change int64) bool {
			changes[accountID].Value += change
			changes[accountID].UpdateTime = index

			return true
		})
	}
	return changes
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
	b.latestCommittedSlot = slotIndex

	return bicRoot, nil
}

func (b *BICManager) commitBICTree(index iotago.SlotIndex, changes map[iotago.AccountID]int64) (bicRoot iotago.Identifier, err error) {
	// update the bic tree to latestCommitted slot index
	for accountID, valueChange := range changes {
		loadedAccount, exists := b.bicTree.Get(accountID)
		if !exists {
			loadedAccount = accounts.NewAccount(b.API(), accountID, accounts.NewCredits(0, index))
		}
		loadedAccount.Credits().Update(valueChange, index)
		b.bicTree.Set(accountID, loadedAccount)
	}
	b.latestCommittedSlot = index

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
