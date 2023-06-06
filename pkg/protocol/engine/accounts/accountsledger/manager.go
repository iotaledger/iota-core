package accountsledger

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

// Manager is a Block Issuer Credits module responsible for tracking block issuance credit balances.
type Manager struct {
	api iotago.API
	// blockBurns keep tracks of the blocks issues up to the LatestCommittedSlot. They are used to deduct the burned
	// amount from the account's credits upon slot commitment.
	blockBurns *shrinkingmap.ShrinkingMap[iotago.SlotIndex, *advancedset.AdvancedSet[iotago.BlockID]]
	// TODO: add in memory shrink version of the slot diffs

	// latestCommittedSlot is where the Account tree is kept at.
	latestCommittedSlot iotago.SlotIndex

	// TODO: on reading from the snapshot: create the Account tree from the account vector and the slot diffs
	// accountsTree represents the Block Issuer data vector for all registered accounts that have a block issuer feature
	// at the latest committed slot, it is updated on the slot commitment.
	accountsTree *ads.Map[iotago.AccountID, accounts.AccountData, *iotago.AccountID, *accounts.AccountData]

	// slot diffs for the Account between [LatestCommitedSlot - MCA, LatestCommitedSlot].
	slotDiff func(iotago.SlotIndex) *prunable.AccountDiffs

	// block is a function that returns a block from the cache or from the database.
	block func(id iotago.BlockID) (*blocks.Block, bool)

	// TODO: properly lock across methods
	mutex sync.RWMutex

	module.Module
}

func New(blockFunc func(id iotago.BlockID) (*blocks.Block, bool), slotDiffFunc func(iotago.SlotIndex) *prunable.AccountDiffs, accountsStore kvstore.KVStore, api iotago.API) *Manager {
	return &Manager{
		api:          api,
		blockBurns:   shrinkingmap.New[iotago.SlotIndex, *advancedset.AdvancedSet[iotago.BlockID]](),
		accountsTree: ads.NewMap[iotago.AccountID, accounts.AccountData](accountsStore),
		block:        blockFunc,
		slotDiff:     slotDiffFunc,
	}
}

func (b *Manager) SetLatestCommittedSlot(index iotago.SlotIndex) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	b.latestCommittedSlot = index
}

// TrackBlock adds the block to the blockBurns set to deduct the burn from credits upon slot commitment.
func (b *Manager) TrackBlock(block *blocks.Block) {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	set, _ := b.blockBurns.GetOrCreate(block.ID().Index(), func() *advancedset.AdvancedSet[iotago.BlockID] {
		return advancedset.New[iotago.BlockID]()
	})
	set.Add(block.ID())
}

// AccountsTreeRoot returns the root of the Account tree with all the accounts ledger data.
func (b *Manager) AccountsTreeRoot() iotago.Identifier {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	return iotago.Identifier(b.accountsTree.Root())
}

// CommitSlot applies the given accountDiffChanges to the Account tree and returns the new accountRoot.
func (b *Manager) CommitSlot(
	slotIndex iotago.SlotIndex,
	accountDiffs map[iotago.AccountID]*prunable.AccountDiff,
	destroyedAccounts *advancedset.AdvancedSet[iotago.AccountID],
) (accountsTreeRoot iotago.Identifier, err error) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	// sanity check if the slotIndex is the next slot to commit
	if slotIndex != b.latestCommittedSlot+1 {
		return iotago.Identifier{}, errors.Errorf("cannot apply the ned diff, there is a gap in committed slots, account vector index: %d, slot to commit: %d", b.latestCommittedSlot, slotIndex)
	}

	// load blocks burned in this slot
	burns, err := b.ComputeBlockBurnsForSlot(slotIndex)
	if err != nil {
		return iotago.Identifier{}, errors.Wrap(err, "could not create block burns for slot")
	}
	// apply the burns to the accountDiffs
	for id, burn := range burns {
		accountDiffs[id].Change -= int64(burn)
	}

	// store the diff and apply it to the account vector tree, obtaining the new root
	if accountsTreeRoot, err = b.applyDiff(slotIndex, accountDiffs, destroyedAccounts); err != nil {
		return iotago.Identifier{}, errors.Wrap(err, "could not apply diff to account tree")
	}

	// set the index where the tree is now at
	b.latestCommittedSlot = slotIndex

	// TODO: when to exactly evict?
	b.evict(slotIndex - iotago.MaxCommitableSlotAge - 1)

	return accountsTreeRoot, nil
}

func (b *Manager) ComputeBlockBurnsForSlot(slotIndex iotago.SlotIndex) (burns map[iotago.AccountID]uint64, err error) {
	burns = make(map[iotago.AccountID]uint64)
	if set, exists := b.blockBurns.Get(slotIndex); exists {
		for it := set.Iterator(); it.HasNext(); {
			blockID := it.Next()
			block, blockLoaded := b.block(blockID)
			if !blockLoaded {
				return nil, errors.Errorf("cannot apply the new diff, block %s not found in the block cache", blockID)
			}
			burns[block.Block().IssuerID] += block.Block().BurnedMana
		}
	}
	return burns, nil
}

// Account loads the account's data at a specific slot index.
func (b *Manager) Account(accountID iotago.AccountID, targetIndex iotago.SlotIndex) (account accounts.Account, exists bool, err error) {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	if targetIndex < b.latestCommittedSlot-iotago.MaxCommitableSlotAge {
		return nil, false, fmt.Errorf("can't calculate account, slot index older than accountIndex (%d<%d)", targetIndex, b.latestCommittedSlot)
	}
	if targetIndex > b.latestCommittedSlot {
		return nil, false, fmt.Errorf("can't retrieve account, slot %d is not committed yet, lastest committed slot: %d", targetIndex, b.latestCommittedSlot)
	}

	// read initial account data at the latest committed slot
	loadedAccount, exists := b.accountsTree.Get(accountID)
	if !exists {
		loadedAccount = accounts.NewAccountData(b.api, accountID, accounts.NewBlockIssuanceCredits(0, targetIndex), loadedAccount.OutputID())
	}
	wasDestroyed, err := b.rollbackAccountTo(loadedAccount, targetIndex)
	if err != nil {
		return nil, false, err
	}

	// account not present in the account and it was not marked as destroyed in slots between targetIndex and latestCommittedSlot
	if !exists && !wasDestroyed {
		return nil, false, nil
	}

	return loadedAccount, true, nil
}

func (b *Manager) rollbackAccountTo(accountData *accounts.AccountData, targetIndex iotago.SlotIndex) (wasDestroyed bool, err error) {
	// to reach targetIndex, we need to rollback diffs from the current latestCommittedSlot down to targetIndex + 1
	for diffIndex := b.latestCommittedSlot; diffIndex > targetIndex; diffIndex-- {
		diffStore := b.slotDiff(diffIndex)
		if diffStore == nil {
			return false, errors.Errorf("can't retrieve account, could not find diff store for slot (%d)", diffIndex)
		}

		found, err := diffStore.Has(accountData.ID())
		if err != nil {
			return false, errors.Wrapf(err, "can't retrieve account, could not check if diff store for slot (%d) has account (%s)", diffIndex, accountData.ID())
		}

		// no changes for this account in this slot
		if !found {
			continue
		}

		diffChange, destroyed, err := diffStore.Load(accountData.ID())
		if err != nil {
			return false, errors.Wrapf(err, "can't retrieve account, could not load diff for account (%s) in slot (%d)", accountData.ID(), diffIndex)
		}

		// update the account data with the diff
		accountData.BlockIssuanceCredits().Update(-diffChange.Change, diffChange.PreviousUpdatedTime)
		accountData.AddPublicKey(diffChange.PubKeysRemoved...)
		accountData.RemovePublicKey(diffChange.PubKeysAdded...)

		// collected to see if account was destroyed between slotIndex and b.latestCommittedSlot index.
		wasDestroyed = wasDestroyed || destroyed
	}

	return wasDestroyed, nil
}

func (b *Manager) Shutdown() {
	b.TriggerStopped()
}

func (b *Manager) applyDiff(slotIndex iotago.SlotIndex, accountDiffs map[iotago.AccountID]*prunable.AccountDiff, destroyedAccounts *advancedset.AdvancedSet[iotago.AccountID]) (iotago.Identifier, error) {
	// load diffs storage for the slot
	diffStore := b.slotDiff(slotIndex)
	for accountID, accountDiff := range accountDiffs {
		err := diffStore.Store(accountID, *accountDiff, destroyedAccounts.Has(accountID))
		if err != nil {
			return iotago.Identifier{}, errors.Wrapf(err, "could not store diff to slot %d", slotIndex)
		}
	}

	accountRoot, err := b.commitAccountTree(slotIndex, accountDiffs, destroyedAccounts)
	if err != nil {
		return iotago.Identifier{}, errors.Wrapf(err, "could not apply diff to slot %d", slotIndex)
	}

	return accountRoot, nil
}

func (b *Manager) commitAccountTree(index iotago.SlotIndex, accountDiffChanges map[iotago.AccountID]*prunable.AccountDiff, destroyedAccounts *advancedset.AdvancedSet[iotago.AccountID]) (accountRoot iotago.Identifier, err error) {
	// update the account tree to latestCommitted slot index
	for accountID, diffChange := range accountDiffChanges {
		// remove destroyed account, no need to update with diffs
		if destroyedAccounts.Has(accountID) {
			b.accountsTree.Delete(accountID)
			continue
		}

		accountData, exists := b.accountsTree.Get(accountID)
		if !exists {
			accountData = accounts.NewAccountData(b.api, accountID, accounts.NewBlockIssuanceCredits(0, index), accountData.OutputID())
		}
		accountData.BlockIssuanceCredits().Update(diffChange.Change, index)
		accountData.AddPublicKey(diffChange.PubKeysAdded...)
		accountData.RemovePublicKey(diffChange.PubKeysRemoved...)
		b.accountsTree.Set(accountID, accountData)
	}

	return b.AccountsTreeRoot(), nil
}

func (b *Manager) evict(index iotago.SlotIndex) {
	b.blockBurns.Delete(index)
}
