package accountsledger

import (
	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/core/memstorage"
	"github.com/iotaledger/hive.go/core/safemath"
	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger"
	"github.com/iotaledger/iota-core/pkg/storage/prunable/slotstore"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

// Manager is a Block Issuer Credits module responsible for tracking block issuance credit balances.
type Manager struct {
	apiProvider api.Provider
	// blockBurns keep tracks of the block issues up to the LatestCommittedSlot. They are used to deduct the burned
	// amount from the account's credits upon slot commitment.
	blockBurns *shrinkingmap.ShrinkingMap[iotago.SlotIndex, ds.Set[iotago.BlockID]]

	// latestSupportedVersionSignals keep tracks of the latest supported protocol versions supported by validators.
	latestSupportedVersionSignals *memstorage.IndexedStorage[iotago.SlotIndex, iotago.AccountID, *model.SignaledBlock]

	// latestCommittedSlot is where the Account tree is kept at.
	latestCommittedSlot iotago.SlotIndex

	// accountsTree represents the Block Issuer data vector for all registered accounts that have a block issuer feature
	// at the latest committed slot, it is updated on the slot commitment.
	accountsTree ads.Map[iotago.AccountID, *accounts.AccountData]

	// TODO: add in memory shrink version of the slot diffs
	// slot diffs for the Account between [LatestCommittedSlot - MCA, LatestCommittedSlot].
	slotDiff func(iotago.SlotIndex) (*slotstore.AccountDiffs, error)

	// block is a function that returns a block from the cache or from the database.
	block func(id iotago.BlockID) (*blocks.Block, bool)

	mutex syncutils.RWMutex

	module.Module
}

func New(
	apiProvider api.Provider,
	blockFunc func(id iotago.BlockID) (*blocks.Block, bool),
	slotDiffFunc func(iotago.SlotIndex) (*slotstore.AccountDiffs, error),
	accountsStore kvstore.KVStore,
) *Manager {
	return &Manager{
		apiProvider:                   apiProvider,
		blockBurns:                    shrinkingmap.New[iotago.SlotIndex, ds.Set[iotago.BlockID]](),
		latestSupportedVersionSignals: memstorage.NewIndexedStorage[iotago.SlotIndex, iotago.AccountID, *model.SignaledBlock](),
		accountsTree: ads.NewMap(accountsStore,
			iotago.Identifier.Bytes,
			iotago.IdentifierFromBytes,
			(*accounts.AccountData).Bytes,
			func(bytes []byte) (object *accounts.AccountData, consumed int, err error) {
				a := new(accounts.AccountData)
				consumed, err = a.FromBytes(bytes)

				return a, consumed, err
			}),
		block:    blockFunc,
		slotDiff: slotDiffFunc,
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

// TrackBlock adds the block to the blockBurns set to deduct the burn from credits upon slot commitment and updates latest supported version of a validator block.
func (m *Manager) TrackBlock(block *blocks.Block) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	set, _ := m.blockBurns.GetOrCreate(block.ID().Index(), func() ds.Set[iotago.BlockID] {
		return ds.NewSet[iotago.BlockID]()
	})
	set.Add(block.ID())

	if validationBlock, isValidationBlock := block.ValidationBlock(); isValidationBlock {
		newSignaledBlock := model.NewSignaledBlock(block.ID(), block.ProtocolBlock(), validationBlock)

		m.latestSupportedVersionSignals.Get(block.ID().Index(), true).Compute(block.ProtocolBlock().IssuerID, func(currentValue *model.SignaledBlock, exists bool) *model.SignaledBlock {
			if !exists {
				return newSignaledBlock
			}

			if newSignaledBlock.Compare(currentValue) == 1 {
				return newSignaledBlock
			}

			return currentValue
		})
	}
}

func (m *Manager) LoadSlotDiff(index iotago.SlotIndex, accountID iotago.AccountID) (*model.AccountDiff, bool, error) {
	s, err := m.slotDiff(index)
	if err != nil {
		return nil, false, ierrors.Errorf("slot %d already pruned", index)
	}

	accDiff, destroyed, err := s.Load(accountID)
	if err != nil {
		return nil, false, ierrors.Wrapf(err, "failed to load slot diff for account %s", accountID)
	}

	return accDiff, destroyed, nil
}

// AccountsTreeRoot returns the root of the Account tree with all the account ledger data.
func (m *Manager) AccountsTreeRoot() iotago.Identifier {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	return iotago.Identifier(m.accountsTree.Root())
}

// ApplyDiff applies the given accountDiff to the Account tree.
func (m *Manager) ApplyDiff(
	slotIndex iotago.SlotIndex,
	rmc iotago.Mana,
	accountDiffs map[iotago.AccountID]*model.AccountDiff,
	destroyedAccounts ds.Set[iotago.AccountID],
) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// sanity-check if the slotIndex is the next slot to commit
	if slotIndex != m.latestCommittedSlot+1 {
		return ierrors.Errorf("cannot apply the next diff, there is a gap in committed slots, account vector index: %d, slot to commit: %d", m.latestCommittedSlot, slotIndex)
	}

	// load blocks burned in this slot
	if err := m.updateSlotDiffWithBurns(slotIndex, accountDiffs, rmc); err != nil {
		return ierrors.Wrap(err, "could not update slot diff with burns")
	}

	if err := m.updateSlotDiffWithVersionSignals(slotIndex, accountDiffs); err != nil {
		return ierrors.Wrap(err, "could not update slot diff latest version signals")
	}

	for accountID := range accountDiffs {
		destroyed := destroyedAccounts.Has(accountID)
		if destroyed {
			reconstructedAccountDiff, err := m.preserveDestroyedAccountData(accountID)
			if err != nil {
				return ierrors.Wrapf(err, "could not preserve destroyed account data for account %s", accountID)
			}

			accountDiffs[accountID] = reconstructedAccountDiff
		}
	}

	// committing the tree will modify the accountDiffs to take into account the decayed credits
	if err := m.commitAccountTree(slotIndex, accountDiffs, destroyedAccounts); err != nil {
		return ierrors.Wrap(err, "could not commit account tree")
	}

	for accountID, accountDiff := range accountDiffs {
		s, err := m.slotDiff(slotIndex)
		if err != nil {
			return ierrors.Wrapf(err, "could not load slot diff for slot %d", slotIndex)
		}

		err = s.Store(accountID, accountDiff, destroyedAccounts.Has(accountID))
		if err != nil {
			return ierrors.Wrapf(err, "could not store diff to slot %d", slotIndex)
		}
	}

	// set the index where the tree is now at
	m.latestCommittedSlot = slotIndex

	m.evict(slotIndex - m.apiProvider.APIForSlot(slotIndex).ProtocolParameters().MaxCommittableAge() - 1)

	return nil
}

// Account loads the account's data at a specific slot index.
func (m *Manager) Account(accountID iotago.AccountID, targetIndex iotago.SlotIndex) (accountData *accounts.AccountData, exists bool, err error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	return m.account(accountID, targetIndex)

}

func (m *Manager) account(accountID iotago.AccountID, targetIndex iotago.SlotIndex) (accountData *accounts.AccountData, exists bool, err error) {
	// if m.latestCommittedSlot < maxCommittableAge we should have all history
	maxCommittableAge := m.apiProvider.APIForSlot(targetIndex).ProtocolParameters().MaxCommittableAge()
	if m.latestCommittedSlot >= maxCommittableAge && targetIndex+maxCommittableAge < m.latestCommittedSlot {
		return nil, false, ierrors.Errorf("can't calculate account, target slot index older than allowed (%d<%d)", targetIndex, m.latestCommittedSlot-maxCommittableAge)
	}

	if targetIndex > m.latestCommittedSlot {
		return nil, false, ierrors.Errorf("can't retrieve account, slot %d is not committed yet, latest committed slot: %d", targetIndex, m.latestCommittedSlot)
	}

	// read initial account data at the latest committed slot
	loadedAccount, exists, err := m.accountsTree.Get(accountID)
	if err != nil {
		return nil, false, ierrors.Wrapf(err, "can't retrieve account, could not load account (%s) from accounts tree", accountID)
	}

	if !exists {
		loadedAccount = accounts.NewAccountData(accountID, accounts.WithCredits(accounts.NewBlockIssuanceCredits(0, targetIndex)))
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

// PastAccounts loads the past accounts' data at a specific slot index.
func (m *Manager) PastAccounts(accountIDs iotago.AccountIDs, targetIndex iotago.SlotIndex) (pastAccounts map[iotago.AccountID]*accounts.AccountData, err error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	result := make(map[iotago.AccountID]*accounts.AccountData)

	for _, accountID := range accountIDs {
		// read initial account data at the latest committed slot
		loadedAccount, exists, err := m.accountsTree.Get(accountID)
		if err != nil {
			return nil, ierrors.Wrapf(err, "can't retrieve account, could not load account (%s) from accounts tree", accountID)
		}

		if !exists {
			loadedAccount = accounts.NewAccountData(accountID, accounts.WithCredits(accounts.NewBlockIssuanceCredits(0, targetIndex)))
		}
		wasDestroyed, err := m.rollbackAccountTo(loadedAccount, targetIndex)
		if err != nil {
			continue
		}

		// account not present in the accountsTree, and it was not marked as destroyed in slots between targetIndex and latestCommittedSlot
		if !exists && !wasDestroyed {
			continue
		}

		result[accountID] = loadedAccount
	}

	return result, nil
}

func (m *Manager) Rollback(targetIndex iotago.SlotIndex) error {
	for index := m.latestCommittedSlot; index > targetIndex; index-- {
		slotDiff := lo.PanicOnErr(m.slotDiff(index))
		var internalErr error

		if err := slotDiff.Stream(func(accountID iotago.AccountID, accountDiff *model.AccountDiff, destroyed bool) bool {
			accountData, exists, err := m.accountsTree.Get(accountID)
			if err != nil {
				internalErr = ierrors.Wrapf(err, "unable to retrieve account %s to rollback in slot %d", accountID, index)

				return false
			}

			if !exists {
				accountData = accounts.NewAccountData(accountID)
			}

			if _, err := m.rollbackAccountTo(accountData, targetIndex); err != nil {
				internalErr = ierrors.Wrapf(err, "unable to rollback account %s to target slot index %d", accountID, targetIndex)

				return false
			}

			if err := m.accountsTree.Set(accountID, accountData); err != nil {
				internalErr = ierrors.Wrapf(err, "failed to save rolled back account %s to target slot index %d", accountID, targetIndex)

				return false
			}

			return true
		}); err != nil {
			return ierrors.Wrapf(err, "error in streaming account diffs for slot %s", index)
		}

		if internalErr != nil {
			return ierrors.Wrapf(internalErr, "error in rolling back account for slot %s", index)
		}
	}

	return nil
}

// AddAccount adds a new account to the Account tree, allotting to it the balance on the given output.
// The Account will be created associating the given output as the latest state of the account.
func (m *Manager) AddAccount(output *utxoledger.Output, blockIssuanceCredits iotago.BlockIssuanceCredits) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	accountOutput, ok := output.Output().(*iotago.AccountOutput)
	if !ok {
		return ierrors.New("can't add account, output is not an account output")
	}

	var stakingOpts []options.Option[accounts.AccountData]
	if stakingFeature := accountOutput.FeatureSet().Staking(); stakingFeature != nil {
		stakingOpts = append(stakingOpts,
			accounts.WithValidatorStake(stakingFeature.StakedAmount),
			accounts.WithStakeEndEpoch(stakingFeature.EndEpoch),
			accounts.WithFixedCost(stakingFeature.FixedCost),
		)
	}

	accountData := accounts.NewAccountData(
		accountOutput.AccountID,
		append(
			stakingOpts,
			accounts.WithCredits(accounts.NewBlockIssuanceCredits(blockIssuanceCredits, m.latestCommittedSlot)),
			accounts.WithOutputID(output.OutputID()),
			accounts.WithBlockIssuerKeys(accountOutput.FeatureSet().BlockIssuer().BlockIssuerKeys...),
			accounts.WithExpirySlot(accountOutput.FeatureSet().BlockIssuer().ExpirySlot),
		)...,
	)

	if err := m.accountsTree.Set(accountOutput.AccountID, accountData); err != nil {
		return ierrors.Wrapf(err, "can't add account, could not set account (%s) in accounts tree", accountOutput.AccountID)
	}

	if err := m.accountsTree.Commit(); err != nil {
		return ierrors.Wrapf(err, "can't add account (%s), could not commit accounts tree", accountOutput.AccountID)
	}

	return nil
}

func (m *Manager) rollbackAccountTo(accountData *accounts.AccountData, targetIndex iotago.SlotIndex) (wasDestroyed bool, err error) {
	// to reach targetIndex, we need to rollback diffs from the current latestCommittedSlot down to targetIndex + 1
	for diffIndex := m.latestCommittedSlot; diffIndex > targetIndex; diffIndex-- {
		diffStore, err := m.slotDiff(diffIndex)
		if err != nil {
			return false, ierrors.Errorf("can't retrieve account, could not find diff store for slot (%d)", diffIndex)
		}

		found, err := diffStore.Has(accountData.ID)
		if err != nil {
			return false, ierrors.Wrapf(err, "can't retrieve account, could not check if diff store for slot (%d) has account (%s)", diffIndex, accountData.ID)
		}

		// no changes for this account in this slot
		if !found {
			continue
		}

		diffChange, destroyed, err := diffStore.Load(accountData.ID)
		if err != nil {
			return false, ierrors.Wrapf(err, "can't retrieve account, could not load diff for account (%s) in slot (%d)", accountData.ID, diffIndex)
		}

		// update the account data with the diff
		accountData.Credits.Update(-diffChange.BICChange, diffChange.PreviousUpdatedTime)
		// update the expiry slot of the account if it was changed
		if diffChange.PreviousExpirySlot != diffChange.NewExpirySlot {
			accountData.ExpirySlot = diffChange.PreviousExpirySlot
		}
		// update the outputID only if the account got actually transitioned, not if it was only an allotment target
		if diffChange.PreviousOutputID != iotago.EmptyOutputID {
			accountData.OutputID = diffChange.PreviousOutputID
		}
		accountData.AddBlockIssuerKeys(diffChange.BlockIssuerKeysRemoved...)
		accountData.RemoveBlockIssuerKey(diffChange.BlockIssuerKeysAdded...)

		validatorStake, err := safemath.SafeSub(int64(accountData.ValidatorStake), diffChange.ValidatorStakeChange)
		if err != nil {
			return false, ierrors.Wrapf(err, "can't retrieve account, validator stake underflow for account (%s) in slot (%d): %d - %d", accountData.ID, diffIndex, accountData.ValidatorStake, diffChange.ValidatorStakeChange)
		}
		accountData.ValidatorStake = iotago.BaseToken(validatorStake)

		delegationStake, err := safemath.SafeSub(int64(accountData.DelegationStake), diffChange.DelegationStakeChange)
		if err != nil {
			return false, ierrors.Wrapf(err, "can't retrieve account, delegation stake underflow for account (%s) in slot (%d): %d - %d", accountData.ID, diffIndex, accountData.DelegationStake, diffChange.DelegationStakeChange)
		}
		accountData.DelegationStake = iotago.BaseToken(delegationStake)

		stakeEpochEnd, err := safemath.SafeSub(int64(accountData.StakeEndEpoch), diffChange.StakeEndEpochChange)
		if err != nil {
			return false, ierrors.Wrapf(err, "can't retrieve account, stake end epoch underflow for account (%s) in slot (%d): %d - %d", accountData.ID, diffIndex, accountData.StakeEndEpoch, diffChange.StakeEndEpochChange)
		}
		accountData.StakeEndEpoch = iotago.EpochIndex(stakeEpochEnd)

		fixedCost, err := safemath.SafeSub(int64(accountData.FixedCost), diffChange.FixedCostChange)
		if err != nil {
			return false, ierrors.Wrapf(err, "can't retrieve account, fixed cost underflow for account (%s) in slot (%d): %d - %d", accountData.ID, diffIndex, accountData.FixedCost, diffChange.FixedCostChange)
		}
		accountData.FixedCost = iotago.Mana(fixedCost)
		if diffChange.PrevLatestSupportedVersionAndHash != diffChange.NewLatestSupportedVersionAndHash {
			accountData.LatestSupportedProtocolVersionAndHash = diffChange.PrevLatestSupportedVersionAndHash
		}

		// collected to see if an account was destroyed between slotIndex and b.latestCommittedSlot index.
		wasDestroyed = wasDestroyed || destroyed
	}

	return wasDestroyed, nil
}

func (m *Manager) preserveDestroyedAccountData(accountID iotago.AccountID) (accountDiff *model.AccountDiff, err error) {
	// if any data is left on the account, we need to store in the diff, to be able to rollback
	accountData, exists, err := m.accountsTree.Get(accountID)
	if err != nil {
		return nil, ierrors.Wrapf(err, "can't retrieve account, could not load account (%s) from accounts tree", accountID)
	}

	if !exists {
		return nil, err
	}

	// it does not matter if there are any changes in this slot, as the account was destroyed anyway and the data was lost
	// we store the accountState in the form of a diff, so we can roll back to the previous state
	slotDiff := model.NewAccountDiff()
	slotDiff.BICChange = -accountData.Credits.Value
	slotDiff.NewExpirySlot = iotago.SlotIndex(0)
	slotDiff.PreviousExpirySlot = accountData.ExpirySlot
	slotDiff.NewOutputID = iotago.EmptyOutputID
	slotDiff.PreviousOutputID = accountData.OutputID
	slotDiff.PreviousUpdatedTime = accountData.Credits.UpdateTime
	slotDiff.BlockIssuerKeysRemoved = accountData.BlockIssuerKeys.ToSlice()

	slotDiff.ValidatorStakeChange = -int64(accountData.ValidatorStake)
	slotDiff.DelegationStakeChange = -int64(accountData.DelegationStake)
	slotDiff.StakeEndEpochChange = -int64(accountData.StakeEndEpoch)
	slotDiff.FixedCostChange = -int64(accountData.FixedCost)
	slotDiff.NewLatestSupportedVersionAndHash = model.VersionAndHash{}
	slotDiff.PrevLatestSupportedVersionAndHash = accountData.LatestSupportedProtocolVersionAndHash

	return slotDiff, err
}

func (m *Manager) computeBlockBurnsForSlot(slotIndex iotago.SlotIndex, rmc iotago.Mana) (burns map[iotago.AccountID]iotago.Mana, err error) {
	burns = make(map[iotago.AccountID]iotago.Mana)
	validationBlockCount := make(map[iotago.AccountID]int)
	apiForSlot := m.apiProvider.APIForSlot(slotIndex)
	if set, exists := m.blockBurns.Get(slotIndex); exists {
		for it := set.Iterator(); it.HasNext(); {
			blockID := it.Next()
			block, blockLoaded := m.block(blockID)
			if !blockLoaded {
				return nil, ierrors.Errorf("cannot apply the new diff, block %s not found in the block cache", blockID)
			}
			if _, isBasicBlock := block.BasicBlock(); isBasicBlock {
				burns[block.ProtocolBlock().IssuerID] += iotago.Mana(block.WorkScore()) * rmc
			} else if _, isValidationBlock := block.ValidationBlock(); isValidationBlock {
				validationBlockCount[block.ProtocolBlock().IssuerID]++
			}
		}
		validationBlocksPerSlot := int(apiForSlot.ProtocolParameters().ValidationBlocksPerSlot())
		for accountID, count := range validationBlockCount {
			if count > validationBlocksPerSlot {
				// penalize over-issuance
				accountData, exists, err := m.account(accountID, m.latestCommittedSlot)
				if !exists {
					return nil, ierrors.Wrapf(err, "cannot compute penalty for over-issuing validator, account %s could not be retrieved", accountID)
				}
				punishmentEpochs := apiForSlot.ProtocolParameters().PunishmentEpochs()
				manaPunishment, err := apiForSlot.ManaDecayProvider().ManaGenerationWithDecay(accountData.ValidatorStake, slotIndex, slotIndex+apiForSlot.TimeProvider().EpochDurationSlots()*iotago.SlotIndex(punishmentEpochs))
				if err != nil {
					return nil, ierrors.Wrapf(err, "cannot compute penalty for over-issuing validator with account ID %s due to problem with mana generation", accountID)
				}
				burns[accountID] += iotago.Mana(count-validationBlocksPerSlot) * manaPunishment
			}
		}
	}

	return burns, nil
}

func (m *Manager) commitAccountTree(index iotago.SlotIndex, accountDiffChanges map[iotago.AccountID]*model.AccountDiff, destroyedAccounts ds.Set[iotago.AccountID]) error {
	// update the account tree to latestCommitted slot index
	for accountID, diffChange := range accountDiffChanges {
		// remove a destroyed account, no need to update with diffs
		if destroyedAccounts.Has(accountID) {
			if _, err := m.accountsTree.Delete(accountID); err != nil {
				return ierrors.Wrapf(err, "could not delete account (%s) from accounts tree", accountID)
			}

			continue
		}

		accountData, exists, err := m.accountsTree.Get(accountID)
		if err != nil {
			return ierrors.Wrapf(err, "can't retrieve account, could not load account (%s) from accounts tree", accountID)
		}

		if !exists {
			accountData = accounts.NewAccountData(accountID)
		}

		if diffChange.BICChange != 0 || !exists {
			// decay the credits to the current slot if the account exists
			if exists {
				decayedPreviousCredits, err := m.apiProvider.APIForSlot(index).ManaDecayProvider().ManaWithDecay(iotago.Mana(accountData.Credits.Value), accountData.Credits.UpdateTime, index)
				if err != nil {
					return ierrors.Wrapf(err, "can't retrieve account, could not decay credits for account (%s) in slot (%d)", accountData.ID, index)
				}

				// update the account data diff taking into account the decay, the modified diff will be stored in the calling
				// ApplyDiff function to be able to properly rollback the account to a previous slot.
				diffChange.BICChange -= accountData.Credits.Value - iotago.BlockIssuanceCredits(decayedPreviousCredits)
			}

			accountData.Credits.Update(diffChange.BICChange, index)
		}

		// update the expiry slot of the account if it changed
		if diffChange.PreviousExpirySlot != diffChange.NewExpirySlot {
			accountData.ExpirySlot = diffChange.NewExpirySlot
		}

		// update the outputID only if the account got actually transitioned, not if it was only an allotment target
		if diffChange.NewOutputID != iotago.EmptyOutputID {
			accountData.OutputID = diffChange.NewOutputID
		}

		accountData.AddBlockIssuerKeys(diffChange.BlockIssuerKeysAdded...)
		accountData.RemoveBlockIssuerKey(diffChange.BlockIssuerKeysRemoved...)

		validatorStake, err := safemath.SafeAdd(int64(accountData.ValidatorStake), diffChange.ValidatorStakeChange)
		if err != nil {
			return ierrors.Wrapf(err, "can't retrieve account, validator stake overflow for account (%s) in slot (%d): %d + %d", accountData.ID, index, accountData.ValidatorStake, diffChange.ValidatorStakeChange)
		}
		accountData.ValidatorStake = iotago.BaseToken(validatorStake)

		delegationStake, err := safemath.SafeAdd(int64(accountData.DelegationStake), diffChange.DelegationStakeChange)
		if err != nil {
			return ierrors.Wrapf(err, "can't retrieve account, delegation stake overflow for account (%s) in slot (%d): %d + %d", accountData.ID, index, accountData.DelegationStake, diffChange.DelegationStakeChange)
		}
		accountData.DelegationStake = iotago.BaseToken(delegationStake)

		stakeEndEpoch, err := safemath.SafeAdd(int64(accountData.StakeEndEpoch), diffChange.StakeEndEpochChange)
		if err != nil {
			return ierrors.Wrapf(err, "can't retrieve account, stake end epoch overflow for account (%s) in slot (%d): %d + %d", accountData.ID, index, accountData.StakeEndEpoch, diffChange.StakeEndEpochChange)
		}
		accountData.StakeEndEpoch = iotago.EpochIndex(stakeEndEpoch)

		fixedCost, err := safemath.SafeAdd(int64(accountData.FixedCost), diffChange.FixedCostChange)
		if err != nil {
			return ierrors.Wrapf(err, "can't retrieve account, validator fixed cost overflow for account (%s) in slot (%d): %d + %d", accountData.ID, index, accountData.FixedCost, diffChange.FixedCostChange)
		}
		accountData.FixedCost = iotago.Mana(fixedCost)

		if diffChange.PrevLatestSupportedVersionAndHash != diffChange.NewLatestSupportedVersionAndHash && accountData.LatestSupportedProtocolVersionAndHash.Version < diffChange.NewLatestSupportedVersionAndHash.Version {
			accountData.LatestSupportedProtocolVersionAndHash = diffChange.NewLatestSupportedVersionAndHash
		}

		if err := m.accountsTree.Set(accountID, accountData); err != nil {
			return ierrors.Wrapf(err, "could not set account (%s) in accounts tree", accountID)
		}
	}

	if err := m.accountsTree.Commit(); err != nil {
		return ierrors.Wrap(err, "could not commit account tree")
	}

	return nil
}

func (m *Manager) evict(index iotago.SlotIndex) {
	m.blockBurns.Delete(index)
	m.latestSupportedVersionSignals.Evict(index)
}

func (m *Manager) updateSlotDiffWithBurns(slotIndex iotago.SlotIndex, accountDiffs map[iotago.AccountID]*model.AccountDiff, rmc iotago.Mana) error {
	burns, err := m.computeBlockBurnsForSlot(slotIndex, rmc)
	if err != nil {
		return ierrors.Wrap(err, "could not create block burns for slot")
	}
	for id, burn := range burns {
		accountDiff, exists := accountDiffs[id]
		if !exists {
			accountDiff = model.NewAccountDiff()
		}

		accountDiff.BICChange -= iotago.BlockIssuanceCredits(burn)
		accountDiffs[id] = accountDiff
	}

	return nil
}

func (m *Manager) updateSlotDiffWithVersionSignals(slotIndex iotago.SlotIndex, accountDiffs map[iotago.AccountID]*model.AccountDiff) error {
	signalsStorage := m.latestSupportedVersionSignals.Get(slotIndex)
	if signalsStorage == nil {
		return nil
	}

	for id, signaledBlock := range signalsStorage.AsMap() {
		accountData, exists, err := m.accountsTree.Get(id)
		if err != nil {
			return ierrors.Wrapf(err, "can't retrieve account, could not load account (%s) from accounts tree", id)
		}

		if !exists {
			accountData = accounts.NewAccountData(id)
		}

		accountDiff, exists := accountDiffs[id]
		if !exists {
			accountDiff = model.NewAccountDiff()
		}
		newVersionAndHash := model.VersionAndHash{
			Version: signaledBlock.HighestSupportedVersion,
			Hash:    signaledBlock.ProtocolParametersHash,
		}
		if accountData.LatestSupportedProtocolVersionAndHash != newVersionAndHash &&
			accountData.LatestSupportedProtocolVersionAndHash.Version < newVersionAndHash.Version {
			accountDiff.NewLatestSupportedVersionAndHash = newVersionAndHash
			accountDiff.PrevLatestSupportedVersionAndHash = accountData.LatestSupportedProtocolVersionAndHash
			accountDiffs[id] = accountDiff
		}
	}

	return nil
}
