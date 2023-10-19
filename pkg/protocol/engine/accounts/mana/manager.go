package mana

import (
	"github.com/zyedidia/generic/cache"

	"github.com/iotaledger/hive.go/core/safemath"
	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger"
	iotago "github.com/iotaledger/iota.go/v4"
)

// Manager is used to access stored and potential mana of an account in order.
// For stored Mana added to account, or stored/potential Mana spent, we will update on commitment.
// For potential Mana updates and decay, we update on demand if the Mana vector is accessed (by the scheduler).
type Manager struct {
	apiProvider iotago.APIProvider

	manaVectorCache *cache.Cache[iotago.AccountID, *accounts.Mana]

	accountOutputResolveFunc func(iotago.AccountID, iotago.SlotIndex) (*utxoledger.Output, error)

	accountRetrieveFunc func(iotago.AccountID, iotago.SlotIndex) (*accounts.AccountData, bool, error)

	mutex syncutils.Mutex

	module.Module
}

func NewManager(apiProvider iotago.APIProvider, accountOutputResolveFunc func(iotago.AccountID, iotago.SlotIndex) (*utxoledger.Output, error), accountRetrieveFunc func(iotago.AccountID, iotago.SlotIndex) (*accounts.AccountData, bool, error)) *Manager {
	return &Manager{
		apiProvider:              apiProvider,
		accountOutputResolveFunc: accountOutputResolveFunc,
		accountRetrieveFunc:      accountRetrieveFunc,
		manaVectorCache:          cache.New[iotago.AccountID, *accounts.Mana](10000),
	}
}

func (m *Manager) GetManaOnAccount(accountID iotago.AccountID, slot iotago.SlotIndex) (iotago.Mana, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	apiForSlot := m.apiProvider.APIForSlot(slot)

	mana, exists := m.manaVectorCache.Get(accountID)
	// if entry does not exist or required slot is earlier than the one stored in the cache,
	// then need to calculate it from scratch
	if !exists || mana.UpdateTime() > slot {
		output, err := m.accountOutputResolveFunc(accountID, slot)
		if err != nil {
			return 0, ierrors.Wrapf(err, "failed to resolve AccountOutput for %s in slot %s", accountID, slot)
		}

		mana, err = m.getMana(accountID, output, slot)
		if err != nil {
			return 0, ierrors.Wrapf(err, "failed to calculate mana for %s in slot %s", accountID, slot)
		}

		// If it did not exist in cache, then add an entry to cache.
		// If it did exist and the requested slot is earlier,
		// then don't add it as it could lead to inaccurate results if any later slot is requested afterward.
		if !exists {
			m.manaVectorCache.Put(accountID, mana)
		}
	}

	manaDecayProvider := apiForSlot.ManaDecayProvider()

	// If the requested slot is the same as the update time the cached mana, then return how it is.
	// Otherwise, need to calculate decay and potential mana generation before returning.
	if slot == mana.UpdateTime() {
		return mana.Value(), nil
	}

	// Apply decay to stored, potential and BIC mana that was calculated when adding the entry to cache
	// so that it's correct for the requested slot.
	manaWithDecay, err := manaDecayProvider.ManaWithDecay(mana.Value(), mana.UpdateTime(), slot)
	if err != nil {
		return 0, err
	}

	// Calculate newly generated potential mana since last update time.
	manaPotential, err := manaDecayProvider.ManaGenerationWithDecay(mana.ExcessBaseTokens(), mana.UpdateTime(), slot)
	if err != nil {
		return 0, err
	}

	// Add potential mana to the decayed mana.
	manaWithDecayAndGeneration, err := safemath.SafeAdd(manaWithDecay, manaPotential)
	if err != nil {
		return 0, ierrors.Wrapf(err, "overflow when adding stored and potential mana for account %s", accountID)
	}

	return manaWithDecayAndGeneration, nil
}

func (m *Manager) getMana(accountID iotago.AccountID, output *utxoledger.Output, slot iotago.SlotIndex) (*accounts.Mana, error) {
	bic, bicUpdateTime, err := m.getBIC(accountID, slot)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to retrieve BIC")
	}

	manaDecayProvider := m.apiProvider.APIForSlot(slot).ManaDecayProvider()
	minDeposit := lo.PanicOnErr(m.apiProvider.APIForSlot(slot).StorageScoreStructure().MinDeposit(output.Output()))
	excessBaseTokens, err := safemath.SafeSub(output.BaseTokenAmount(), minDeposit)
	if err != nil {
		// if subtraction underflows, then excess base tokens is 0
		excessBaseTokens = 0
	}

	if output.SlotCreated() > slot || bicUpdateTime > slot {
		return nil, ierrors.Errorf("BIC update time (%d) or output creation slot (%d) later than requested slot (%d)", bicUpdateTime, output.SlotCreated(), slot)
	}

	// Decay and generate stored mana to match either the BIC update time or Output creation slot(bigger of the two),
	// so that subsequent decay calculations don't need to consider the fact that the two values should be decayed for different periods,
	// but instead can be added and decayed together.

	var manaUpdateTime iotago.SlotIndex
	var totalMana iotago.Mana
	if bicUpdateTime > output.SlotCreated() {
		manaPotential, err := manaDecayProvider.ManaGenerationWithDecay(excessBaseTokens, output.SlotCreated(), bicUpdateTime)
		if err != nil {
			return nil, ierrors.Wrapf(err, "failed to calculate mana generation with decay (excessBaseTokens: %d; outputSlotCreated: %d; targetSlot: %d)", excessBaseTokens, output.SlotCreated(), bicUpdateTime)
		}

		manaStored, err := manaDecayProvider.ManaWithDecay(output.StoredMana(), output.SlotCreated(), bicUpdateTime)
		if err != nil {
			return nil, ierrors.Wrapf(err, "failed to calculate mana with decay (storedMana: %d; outputSlotCreated: %d; targetSlot: %d)", output.StoredMana(), output.SlotCreated(), bicUpdateTime)
		}

		manaStoredPotential, err := safemath.SafeAdd(manaStored, manaPotential)
		if err != nil {
			return nil, ierrors.Wrapf(err, "overflow when adding decayed stored and potential mana (storedMana: %d; potentialMana: %d)", manaStored, manaPotential)
		}

		totalMana, err = safemath.SafeAdd(bic, manaStoredPotential)
		if err != nil {
			return nil, ierrors.Wrapf(err, "overflow when adding decayed stored, potential mana and BIC (storedPotentialMana: %d; BIC: %d)", manaStoredPotential, bic)
		}

		manaUpdateTime = bicUpdateTime
	} else if output.SlotCreated() > bicUpdateTime {
		// Decay BIC to match the Output creation time.
		bicWithDecay, err := manaDecayProvider.ManaWithDecay(bic, bicUpdateTime, output.SlotCreated())
		if err != nil {
			return nil, err
		}

		totalMana, err = safemath.SafeAdd(bicWithDecay, output.StoredMana())
		if err != nil {
			return nil, ierrors.Wrapf(err, "overflow when adding stored mana and decayed BIC (storedMana: %d; decayed BIC: %d)", output.StoredMana(), bicWithDecay)
		}

		manaUpdateTime = output.SlotCreated()
	} else {
		totalMana, err = safemath.SafeAdd(bic, output.StoredMana())
		if err != nil {
			return nil, ierrors.Wrapf(err, "overflow when adding stored mana and BIC (storedMana: %d; BIC: %d)", output.StoredMana(), bic)
		}

		manaUpdateTime = output.SlotCreated()
	}

	return accounts.NewMana(totalMana, excessBaseTokens, manaUpdateTime), nil
}

func (m *Manager) ApplyDiff(slot iotago.SlotIndex, destroyedAccounts ds.Set[iotago.AccountID], accountOutputs map[iotago.AccountID]*utxoledger.Output, accountDiffs map[iotago.AccountID]*model.AccountDiff) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	destroyedAccounts.Range(func(accountID iotago.AccountID) {
		m.manaVectorCache.Remove(accountID)
	})

	for accountID, accountDiff := range accountDiffs {
		if _, exists := m.manaVectorCache.Get(accountID); exists {
			// If the account output was spent and a new one was created,
			// or if BIC amount changed, then mana entry in the cache needs to be updated.

			var accountOutput *utxoledger.Output
			// If the account output was updated, then we can use the output directly from the diff;
			// otherwise it needs to be retrieved from the UTXO ledger.
			if output, has := accountOutputs[accountID]; has {
				accountOutput = output
			} else if accountDiff.BICChange != 0 {
				var err error
				if accountOutput, err = m.accountOutputResolveFunc(accountID, slot); err != nil {
					return ierrors.Errorf("failed to resolve AccountOutput for %s in slot %s: %w", accountID, slot, err)
				}
			}

			// If account output was not retrieved, that means that the applied diff affected neither BIC nor AccountOutput.
			if accountOutput != nil {
				mana, err := m.getMana(accountID, accountOutput, slot)
				if err != nil {
					return ierrors.Wrapf(err, "failed to calculate mana on an account %s", accountID)
				}

				m.manaVectorCache.Put(accountID, mana)
			}
		}
	}

	return nil
}

func (m *Manager) getBIC(accountID iotago.AccountID, slot iotago.SlotIndex) (bic iotago.Mana, updateTime iotago.SlotIndex, err error) {
	accountBIC, exists, err := m.accountRetrieveFunc(accountID, slot)
	if err != nil {
		return 0, 0, ierrors.Wrapf(err, "failed to retrieve account data for %s in slot %s", accountID, slot)
	}
	if !exists {
		return 0, 0, ierrors.Errorf("account data for %s in slot %s does not exist", accountID, slot)
	}
	if accountBIC.Credits.Value <= 0 {
		return 0, 0, nil
	}

	return iotago.Mana(accountBIC.Credits.Value), accountBIC.Credits.UpdateTime, nil
}
