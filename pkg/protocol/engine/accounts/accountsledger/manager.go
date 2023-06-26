package accountsledger

import (
	"sync"

	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/crypto/ed25519"
	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger"
	"github.com/iotaledger/iota-core/pkg/storage/prunable"
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

	// accountsTree represents the Block Issuer data vector for all registered accounts that have a block issuer feature
	// at the latest committed slot, it is updated on the slot commitment.
	accountsTree *ads.Map[iotago.AccountID, accounts.AccountData, *iotago.AccountID, *accounts.AccountData]

	// slot diffs for the Account between [LatestCommittedSlot - MCA, LatestCommittedSlot].
	slotDiff func(iotago.SlotIndex) *prunable.AccountDiffs

	// block is a function that returns a block from the cache or from the database.
	block func(id iotago.BlockID) (*blocks.Block, bool)

	maxCommittableAge iotago.SlotIndex

	mutex sync.RWMutex

	module.Module
}

func New(
	blockFunc func(id iotago.BlockID) (*blocks.Block, bool),
	slotDiffFunc func(iotago.SlotIndex) *prunable.AccountDiffs,
	accountsStore kvstore.KVStore,
	api iotago.API,
) *Manager {
	return &Manager{
		api:          api,
		blockBurns:   shrinkingmap.New[iotago.SlotIndex, *advancedset.AdvancedSet[iotago.BlockID]](),
		accountsTree: ads.NewMap[iotago.AccountID, accounts.AccountData](accountsStore),
		block:        blockFunc,
		slotDiff:     slotDiffFunc,
	}
}

func (m *Manager) Shutdown() {
	m.TriggerStopped()
}

func (m *Manager) SetLatestCommittedSlot(index iotago.SlotIndex) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.latestCommittedSlot = index
}

func (m *Manager) SetLivenessThreshold(livenessThreshold iotago.SlotIndex) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.maxCommittableAge = livenessThreshold
}

// TrackBlock adds the block to the blockBurns set to deduct the burn from credits upon slot commitment.
func (m *Manager) TrackBlock(block *blocks.Block) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	set, _ := m.blockBurns.GetOrCreate(block.ID().Index(), func() *advancedset.AdvancedSet[iotago.BlockID] {
		return advancedset.New[iotago.BlockID]()
	})
	set.Add(block.ID())
}

func (m *Manager) LoadSlotDiff(index iotago.SlotIndex, accountID iotago.AccountID) (*prunable.AccountDiff, bool, error) {
	s := m.slotDiff(index)
	if s == nil {
		return nil, false, errors.Errorf("slot %d already pruned", index)
	}

	accDiff, destroyed, err := s.Load(accountID)
	if err != nil {
		return nil, false, errors.Wrapf(err, "failed to load slot diff for account %s", accountID.String())
	}

	return &accDiff, destroyed, nil
}

// AccountsTreeRoot returns the root of the Account tree with all the accounts ledger data.
func (m *Manager) AccountsTreeRoot() iotago.Identifier {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	return iotago.Identifier(m.accountsTree.Root())
}

// ApplyDiff applies the given accountDiff to the Account tree.
func (m *Manager) ApplyDiff(
	slotIndex iotago.SlotIndex,
	accountDiffs map[iotago.AccountID]*prunable.AccountDiff,
	destroyedAccounts *advancedset.AdvancedSet[iotago.AccountID],
) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// sanity-check if the slotIndex is the next slot to commit
	if slotIndex != m.latestCommittedSlot+1 {
		return errors.Errorf("cannot apply the next diff, there is a gap in committed slots, account vector index: %d, slot to commit: %d", m.latestCommittedSlot, slotIndex)
	}

	// load blocks burned in this slot
	burns, err := m.computeBlockBurnsForSlot(slotIndex)
	if err != nil {
		return errors.Wrap(err, "could not create block burns for slot")
	}
	m.updateSlotDiffWithBurns(burns, accountDiffs)
	// store the diff and apply it to the account vector tree, obtaining the new root
	if err = m.applyDiffs(slotIndex, accountDiffs, destroyedAccounts); err != nil {
		return errors.Wrap(err, "could not apply diff to account tree")
	}

	// set the index where the tree is now at
	m.latestCommittedSlot = slotIndex

	// TODO: when to exactly evict?
	m.evict(slotIndex - m.maxCommittableAge - 1)

	return nil
}

// Account loads the account's data at a specific slot index.
func (m *Manager) Account(accountID iotago.AccountID, targetIndex iotago.SlotIndex) (accountData *accounts.AccountData, exists bool, err error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	// if m.latestCommittedSlot < m.maxCommittableAge we should have all history
	if m.latestCommittedSlot >= m.maxCommittableAge && targetIndex < m.latestCommittedSlot-m.maxCommittableAge {
		return nil, false, errors.Errorf("can't calculate account, target slot index older than accountIndex (%d<%d)", targetIndex, m.latestCommittedSlot-m.maxCommittableAge)
	}
	if targetIndex > m.latestCommittedSlot {
		return nil, false, errors.Errorf("can't retrieve account, slot %d is not committed yet, latest committed slot: %d", targetIndex, m.latestCommittedSlot)
	}

	// read initial account data at the latest committed slot
	loadedAccount, exists := m.accountsTree.Get(accountID)

	if !exists {
		loadedAccount = accounts.NewAccountData(accountID, accounts.NewBlockIssuanceCredits(0, targetIndex), iotago.EmptyOutputID)
	}
	wasDestroyed, err := m.rollbackAccountTo(loadedAccount, targetIndex)
	if err != nil {
		return nil, false, err
	}

	// account not present in the accountsTree, and it was not marked as destroyed in slots between targetIndex and latestCommittedSlot
	if !exists && !wasDestroyed {
		return nil, false, nil
	}

	return loadedAccount, true, nil
}

// AddAccount adds a new account to the Account tree, allotting to it the balance on the given output.
// The Account will be created associating the given output as the latest state of the account.
func (m *Manager) AddAccount(output *utxoledger.Output) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	accountOutput, ok := output.Output().(*iotago.AccountOutput)
	if !ok {
		return errors.Errorf("can't add account, output is not an account output")
	}

	accountData := accounts.NewAccountData(
		accountOutput.AccountID,
		accounts.NewBlockIssuanceCredits(int64(accountOutput.Amount), m.latestCommittedSlot),
		output.OutputID(),
		ed25519.NativeToPublicKeys(accountOutput.FeatureSet().BlockIssuer().BlockIssuerKeys)...,
	)

	m.accountsTree.Set(accountOutput.AccountID, accountData)

	return nil
}

func (m *Manager) rollbackAccountTo(accountData *accounts.AccountData, targetIndex iotago.SlotIndex) (wasDestroyed bool, err error) {
	// to reach targetIndex, we need to rollback diffs from the current latestCommittedSlot down to targetIndex + 1
	for diffIndex := m.latestCommittedSlot; diffIndex > targetIndex; diffIndex-- {
		diffStore := m.slotDiff(diffIndex)
		if diffStore == nil {
			return false, errors.Errorf("can't retrieve account, could not find diff store for slot (%d)", diffIndex)
		}

		found, err := diffStore.Has(accountData.ID)
		if err != nil {
			return false, errors.Wrapf(err, "can't retrieve account, could not check if diff store for slot (%d) has account (%s)", diffIndex, accountData.ID)
		}

		// no changes for this account in this slot
		if !found {
			continue
		}

		diffChange, destroyed, err := diffStore.Load(accountData.ID)
		if err != nil {
			return false, errors.Wrapf(err, "can't retrieve account, could not load diff for account (%s) in slot (%d)", accountData.ID, diffIndex)
		}

		// update the account data with the diff
		accountData.Credits.Update(-diffChange.Change, diffChange.PreviousUpdatedTime)
		// update the outputID only if the account got actually transitioned, not if it was only an allotment target
		if diffChange.PreviousOutputID != iotago.EmptyOutputID {
			accountData.OutputID = diffChange.PreviousOutputID
		}
		accountData.AddPublicKeys(diffChange.PubKeysRemoved...)
		accountData.RemovePublicKeys(diffChange.PubKeysAdded...)

		// collected to see if an account was destroyed between slotIndex and b.latestCommittedSlot index.
		wasDestroyed = wasDestroyed || destroyed
	}

	return wasDestroyed, nil
}

func (m *Manager) preserveDestroyedAccountData(accountID iotago.AccountID) *prunable.AccountDiff {
	// if any data is left on the account, we need to store in the diff, to be able to rollback
	accountData, exists := m.accountsTree.Get(accountID)
	if !exists {
		return nil
	}

	// it does not matter if there are any changes in this slot, as the account was destroyed anyway and the data was lost
	// we store the accountState in form of a diff, so we can roll back to the previous state
	slotDiff := prunable.NewAccountDiff()
	slotDiff.Change = -accountData.Credits.Value
	slotDiff.NewOutputID = iotago.OutputID{}
	slotDiff.PreviousOutputID = accountData.OutputID
	slotDiff.PreviousUpdatedTime = accountData.Credits.UpdateTime
	slotDiff.PubKeysRemoved = accountData.PubKeys.Slice()

	return slotDiff
}

func (m *Manager) computeBlockBurnsForSlot(slotIndex iotago.SlotIndex) (burns map[iotago.AccountID]uint64, err error) {
	burns = make(map[iotago.AccountID]uint64)
	if set, exists := m.blockBurns.Get(slotIndex); exists {
		for it := set.Iterator(); it.HasNext(); {
			blockID := it.Next()
			block, blockLoaded := m.block(blockID)
			if !blockLoaded {
				return nil, errors.Errorf("cannot apply the new diff, block %s not found in the block cache", blockID)
			}
			burns[block.Block().IssuerID] += block.Block().BurnedMana
		}
	}

	return burns, nil
}

func (m *Manager) applyDiffs(slotIndex iotago.SlotIndex, accountDiffs map[iotago.AccountID]*prunable.AccountDiff, destroyedAccounts *advancedset.AdvancedSet[iotago.AccountID]) error {
	// Load diffs storage for the slot. The storage can never be nil (pruned) because we are just committing the slot.
	diffStore := m.slotDiff(slotIndex)
	for accountID, accountDiff := range accountDiffs {
		destroyed := destroyedAccounts.Has(accountID)
		if destroyed {
			accountDiff = m.preserveDestroyedAccountData(accountID)
		}
		err := diffStore.Store(accountID, *accountDiff, destroyed)
		if err != nil {
			return errors.Wrapf(err, "could not store diff to slot %d", slotIndex)
		}
	}

	m.commitAccountTree(slotIndex, accountDiffs, destroyedAccounts)

	return nil
}

func (m *Manager) commitAccountTree(index iotago.SlotIndex, accountDiffChanges map[iotago.AccountID]*prunable.AccountDiff, destroyedAccounts *advancedset.AdvancedSet[iotago.AccountID]) {
	// update the account tree to latestCommitted slot index
	for accountID, diffChange := range accountDiffChanges {
		// remove destroyed account, no need to update with diffs
		if destroyedAccounts.Has(accountID) {
			m.accountsTree.Delete(accountID)
			continue
		}

		accountData, exists := m.accountsTree.Get(accountID)
		if !exists {
			accountData = accounts.NewAccountData(accountID, accounts.NewBlockIssuanceCredits(0, 0), iotago.OutputID{})
		}
		accountData.Credits.Update(diffChange.Change, index)
		// update the outputID only if the account got actually transitioned, not if it was only an allotment target
		if diffChange.NewOutputID != iotago.EmptyOutputID {
			accountData.OutputID = diffChange.NewOutputID
		}

		accountData.AddPublicKeys(diffChange.PubKeysAdded...)
		accountData.RemovePublicKeys(diffChange.PubKeysRemoved...)
		m.accountsTree.Set(accountID, accountData)
	}
}

func (m *Manager) evict(index iotago.SlotIndex) {
	m.blockBurns.Delete(index)
}

func (m *Manager) updateSlotDiffWithBurns(burns map[iotago.AccountID]uint64, accountDiffs map[iotago.AccountID]*prunable.AccountDiff) {
	for id, burn := range burns {
		accountDiff, exists := accountDiffs[id]
		if !exists {
			accountDiff = prunable.NewAccountDiff()
			accountDiffs[id] = accountDiff
		}
		accountDiff.Change -= int64(burn)
		accountDiffs[id] = accountDiff
	}
}
