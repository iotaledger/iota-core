package bic

import (
	"fmt"
	"sync"

	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/storage/prunable"

	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/ads"

	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	iotago "github.com/iotaledger/iota.go/v4"
)

// BICManager is a Block Issuer Credits module responsible for tracking block issuance credit balances.
type BICManager struct {
	api iotago.API
	// blockBurns keep tracks of the blocks issues up to the LatestCommittedSlot. They are used to deduct the burned
	// amount from the account's credits upon slot commitment.
	blockBurns *shrinkingmap.ShrinkingMap[iotago.SlotIndex, *advancedset.AdvancedSet[iotago.BlockID]]
	// TODO: add in memory shrink version of the slot diffs

	// latestCommittedSlot is where the BIC tree is kept at.
	latestCommittedSlot iotago.SlotIndex

	// TODO: on reading from the snapshot: create the BIC tree from the bic vector and the slot diffs
	// bic represents the Block Issuer Credits vector of all registered accounts for bicTreeindex slot, it is updated on the slot commitment.
	bicTree *ads.Map[iotago.AccountID, accounts.AccountData, *iotago.AccountID, *accounts.AccountData]

	// slot diffs for the BIC between [LatestCommitedSlot - MCA, LatestCommitedSlot].
	slotDiffFunc func(iotago.SlotIndex) *prunable.BICDiffs

	// blockFunc is a function that returns a block from the cache or from the database.
	blockFunc func(id iotago.BlockID) (*blocks.Block, bool)

	// TODO: properly lock across methods
	mutex sync.RWMutex

	module.Module
}

func New(blockFunc func(id iotago.BlockID) (*blocks.Block, bool), slotDiffFunc func(iotago.SlotIndex) *prunable.BICDiffs, accountsStore kvstore.KVStore, api iotago.API) *BICManager {
	return &BICManager{
		api:          api,
		blockBurns:   shrinkingmap.New[iotago.SlotIndex, *advancedset.AdvancedSet[iotago.BlockID]](),
		bicTree:      ads.NewMap[iotago.AccountID, accounts.AccountData](accountsStore),
		blockFunc:    blockFunc,
		slotDiffFunc: slotDiffFunc,
	}
}

func (b *BICManager) SetBICIndex(index iotago.SlotIndex) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	b.latestCommittedSlot = index
}

// TrackBlock adds the block to the blockBurns set to deduct the burn from credits upon slot commitment.
func (b *BICManager) TrackBlock(block *blocks.Block) {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	set, _ := b.blockBurns.GetOrCreate(block.ID().Index(), func() *advancedset.AdvancedSet[iotago.BlockID] {
		return advancedset.New[iotago.BlockID]()
	})
	set.Add(block.ID())
}

// BICTreeRoot returns the root of the BIC tree with all the accounts ledger data.
func (b *BICManager) BICTreeRoot() iotago.Identifier {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	return iotago.Identifier(b.bicTree.Root())
}

// CommitSlot applies the given bicDiffChanges to the BIC tree and returns the new bicRoot.
func (b *BICManager) CommitSlot(
	slotIndex iotago.SlotIndex,
	bicDiffs map[iotago.AccountID]*prunable.BICDiff,
	destroyedAccounts *advancedset.AdvancedSet[iotago.AccountID],
) (bicRoot iotago.Identifier, err error) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	// sanity check if the slotIndex is the next slot to commit
	if slotIndex != b.latestCommittedSlot+1 {
		return iotago.Identifier{}, errors.Errorf("cannot apply the ned diff, there is a gap in committed slots, bic vector index: %d, slot to commit: %d", b.latestCommittedSlot, slotIndex)
	}

	// load blocks burned in this slot
	burns, err := b.createBlockBurnsForSlot(slotIndex)
	if err != nil {
		return iotago.Identifier{}, errors.Wrap(err, "could not create block burns for slot")
	}
	// apply the burns to the bicDiffs
	for id, burn := range burns {
		bicDiffs[id].Change -= int64(burn)
	}

	// store the diff and apply it to the bic vector tree, obtaining the new root
	if bicRoot, err = b.applyDiff(slotIndex, bicDiffs, destroyedAccounts); err != nil {
		return iotago.Identifier{}, errors.Wrap(err, "could not apply diff to bic tree")
	}

	// set the index where the tree is now at
	b.latestCommittedSlot = slotIndex

	// TODO: when to exactly evict?
	b.evict(slotIndex - iotago.MaxCommitableSlotAge - 1)

	return bicRoot, nil
}

func (b *BICManager) createBlockBurnsForSlot(slotIndex iotago.SlotIndex) (burns map[iotago.AccountID]uint64, err error) {
	burns = make(map[iotago.AccountID]uint64)
	if set, exists := b.blockBurns.Get(slotIndex); exists {
		for it := set.Iterator(); it.HasNext(); {
			blockID := it.Next()
			block, blockLoaded := b.blockFunc(blockID)
			if !blockLoaded {
				return nil, errors.Errorf("cannot apply the new diff, block %s not found in the block cache", blockID)
			}
			burns[block.Block().IssuerID] += block.Block().BurnedMana
		}
	}
	return burns, nil
}

// BIC loads the account's data at a specific slot index.
func (b *BICManager) BIC(accountID iotago.AccountID, slotIndex iotago.SlotIndex) (account accounts.Account, exists bool, err error) {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	if slotIndex < b.latestCommittedSlot-iotago.MaxCommitableSlotAge {
		return nil, false, fmt.Errorf("can't calculate BIC, slot index older than bicIndex (%d<%d)", slotIndex, b.latestCommittedSlot)
	}
	if slotIndex > b.latestCommittedSlot {
		return nil, false, fmt.Errorf("can't retrieve BIC, slot %d is not committed yet, lastest committed slot: %d", slotIndex, b.latestCommittedSlot)
	}

	// read start value from the bic vector at b.bicIndex
	loadedAccount, exists := b.bicTree.Get(accountID)
	if !exists {
		loadedAccount = accounts.NewAccountData(b.api, accountID, accounts.NewCredits(0, slotIndex))
	}
	wasDestroyed, err := b.rollbackAccountTo(slotIndex, accountID, loadedAccount)
	if err != nil {
		return nil, false, err
	}
	// account not present in the BIC and it was not marked as destroyed in slots between slotIndex and b.latestCommittedSlot
	if !exists && !wasDestroyed {
		return nil, false, nil
	}

	return loadedAccount, true, nil
}

func (b *BICManager) rollbackAccountTo(targetIndex iotago.SlotIndex, accountID iotago.AccountID, accountData *accounts.AccountData) (bool, error) {
	wasDestroyed := false
	// last applied diff should be the diff for slotIndex + 1
	for diffIndex := b.latestCommittedSlot; diffIndex > targetIndex; diffIndex-- {
		destroyed, err := b.rollbackAccountOnce(diffIndex, accountID, accountData)
		if err != nil {
			return false, err
		}
		// collected to see if account was destroyed between slotIndex and b.latestCommittedSlot intex.
		wasDestroyed = wasDestroyed || destroyed
	}
	return wasDestroyed, nil
}

func (b *BICManager) rollbackAccountOnce(diffIndex iotago.SlotIndex, accountID iotago.AccountID, accountData *accounts.AccountData) (destroyed bool, err error) {
	diffStore := b.slotDiffFunc(diffIndex)
	if diffStore == nil {
		return false, errors.Errorf("can't retrieve BIC, could not find diff store for slot (%d)", diffIndex)
	}
	found, err := diffStore.Has(accountID)
	if err != nil {
		return false, errors.Wrapf(err, "can't retrieve BIC, could not check if diff store for slot (%d) has account (%s)", diffIndex, accountID)
	}
	if !found {
		// no changes for this account in this slot
		return false, nil
	}
	diffChange, destroyed, err := diffStore.Load(accountID)
	if err != nil {
		return false, errors.Wrapf(err, "can't retrieve BIC, could not load diff for account (%s) in slot (%d)", accountID, diffIndex)
	}
	accountData.Credits().Update(-diffChange.Change, diffChange.PreviousUpdatedTime)
	accountData.AddPublicKey(diffChange.PubKeysRemoved...)
	accountData.RemovePublicKey(diffChange.PubKeysAdded...)
	return destroyed, nil
}

func (b *BICManager) Shutdown() {
	b.TriggerStopped()
}

func (b *BICManager) applyDiff(slotIndex iotago.SlotIndex, bicDiffs map[iotago.AccountID]*prunable.BICDiff, destroyedAccounts *advancedset.AdvancedSet[iotago.AccountID]) (iotago.Identifier, error) {
	// load diffs storage for the slot
	diffStore := b.slotDiffFunc(slotIndex)
	for accountID, bicDiff := range bicDiffs {
		err := diffStore.Store(accountID, *bicDiff, destroyedAccounts.Has(accountID))
		if err != nil {
			return iotago.Identifier{}, errors.Wrapf(err, "could not store diff to slot %d", slotIndex)
		}
	}

	bicRoot, err := b.commitBICTree(slotIndex, bicDiffs, destroyedAccounts)
	if err != nil {
		return iotago.Identifier{}, errors.Wrapf(err, "could not apply diff to slot %d", slotIndex)
	}

	return bicRoot, nil
}

func (b *BICManager) commitBICTree(index iotago.SlotIndex, bicDiffChanges map[iotago.AccountID]*prunable.BICDiff, destroyedAccounts *advancedset.AdvancedSet[iotago.AccountID]) (bicRoot iotago.Identifier, err error) {
	// update the bic tree to latestCommitted slot index
	for accountID, diffChange := range bicDiffChanges {
		// remove destroyed account, no need to update with diffs
		if destroyedAccounts.Has(accountID) {
			b.bicTree.Delete(accountID)
			continue
		}

		accountData, exists := b.bicTree.Get(accountID)
		if !exists {
			accountData = accounts.NewAccountData(b.api, accountID, accounts.NewCredits(0, index), accountData.OutputID())
		}
		accountData.Credits().Update(diffChange.Change, index)
		accountData.AddPublicKey(diffChange.PubKeysAdded...)
		accountData.RemovePublicKey(diffChange.PubKeysRemoved...)
		b.bicTree.Set(accountID, accountData)
	}

	return b.BICTreeRoot(), nil
}

func (b *BICManager) evict(index iotago.SlotIndex) {
	b.blockBurns.Delete(index)
}
