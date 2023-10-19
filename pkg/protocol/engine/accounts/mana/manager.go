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

func (m *Manager) GetManaOnAccount(accountID iotago.AccountID, currentSlot iotago.SlotIndex) (iotago.Mana, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	apiForSlot := m.apiProvider.APIForSlot(currentSlot)

	mana, exists := m.manaVectorCache.Get(accountID)
	if !exists || mana.UpdateTime() > currentSlot {
		output, err := m.accountOutputResolveFunc(accountID, currentSlot)
		if err != nil {
			return 0, ierrors.Errorf("failed to resolve AccountOutput for %s in slot %s: %w", accountID, currentSlot, err)
		}
		minDeposit, err := apiForSlot.StorageScoreStructure().MinDeposit(output.Output())
		if err != nil {
			return 0, ierrors.Errorf("failed to get min deposit for %s: %w", accountID, err)
		}
		excessBaseTokens, err := safemath.SafeSub(output.BaseTokenAmount(), minDeposit)
		if err != nil {
			excessBaseTokens = 0
		}

		decayedBIC, err := m.getDecayedBIC(accountID, currentSlot, apiForSlot)
		if err != nil {
			return 0, ierrors.Wrapf(err, "failed to get decayed BIC for %s", accountID)
		}
		totalMana, err := safemath.SafeAdd(output.StoredMana(), decayedBIC)
		if err != nil {
			return 0, ierrors.Wrapf(err, "overflow when adding stored mana (%d) and decayed BIC (%d) for account %s", output.StoredMana(), decayedBIC, accountID)
		}

		mana = accounts.NewMana(totalMana, excessBaseTokens, output.SlotCreated())

		if !exists {
			m.manaVectorCache.Put(accountID, mana)
		}
	}

	if currentSlot == mana.UpdateTime() {
		return mana.Value(), nil
	}

	manaDecayProvider := apiForSlot.ManaDecayProvider()
	// apply decay to stored, allotted and potential that was added on last update
	manaStored, err := manaDecayProvider.ManaWithDecay(mana.Value(), mana.UpdateTime(), currentSlot)
	if err != nil {
		return 0, err
	}
	updatedValue := manaStored

	// get newly generated potential since last update and apply decay
	manaPotential, err := manaDecayProvider.ManaGenerationWithDecay(mana.ExcessBaseTokens(), mana.UpdateTime(), currentSlot)
	if err != nil {
		return 0, err
	}
	updatedValue, err = safemath.SafeAdd(updatedValue, manaPotential)
	if err != nil {
		return 0, ierrors.Wrapf(err, "overflow when adding stored and potential mana for account %s", accountID)
	}

	decayedBIC, err := m.getDecayedBIC(accountID, currentSlot, apiForSlot)
	if err != nil {
		return 0, ierrors.Wrapf(err, "failed to get decayed BIC for %s", accountID)
	}
	updatedValue, err = safemath.SafeAdd(updatedValue, decayedBIC)
	if err != nil {
		return 0, ierrors.Wrapf(err, "overflow when adding stored, potential and decayed BIC for account %s", accountID)
	}

	mana.UpdateValue(updatedValue, currentSlot)

	return mana.Value(), nil
}

func (m *Manager) ApplyDiff(slot iotago.SlotIndex, destroyedAccounts ds.Set[iotago.AccountID], accountOutputs map[iotago.AccountID]*utxoledger.Output, accountDiffs map[iotago.AccountID]*model.AccountDiff) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	apiForSlot := m.apiProvider.APIForSlot(slot)

	destroyedAccounts.Range(func(accountID iotago.AccountID) {
		m.manaVectorCache.Remove(accountID)
	})

	for accountID := range accountDiffs {
		mana, exists := m.manaVectorCache.Get(accountID)
		if exists {
			var excessBaseTokens iotago.BaseToken
			var storedMana iotago.Mana
			var err error
			if output, has := accountOutputs[accountID]; has {
				minDeposit := lo.PanicOnErr(apiForSlot.StorageScoreStructure().MinDeposit(output.Output()))
				excessBaseTokens, err = safemath.SafeSub(output.BaseTokenAmount(), minDeposit)
				if err != nil {
					excessBaseTokens = 0
				}
				storedMana = output.StoredMana()
			}
			decayedBIC, err := m.getDecayedBIC(accountID, slot, apiForSlot)
			if err != nil {
				return err
			}
			totalMana, err := safemath.SafeAdd(decayedBIC, storedMana)
			if err != nil {
				return ierrors.Wrapf(err, "overflow when adding stored mana and decayed BIC for account %s", accountID)
			}

			mana.Update(totalMana, excessBaseTokens, slot)
		}
	}

	return nil
}

func (m *Manager) getDecayedBIC(accountID iotago.AccountID, slot iotago.SlotIndex, apiForSlot iotago.API) (iotago.Mana, error) {
	accountBIC, exists, err := m.accountRetrieveFunc(accountID, slot)
	if err != nil {
		return 0, ierrors.Wrapf(err, "failed to retrieve account data for %s in slot %s", accountID, slot)
	}
	if !exists {
		return 0, ierrors.Errorf("account data for %s in slot %s does not exist", accountID, slot)
	}
	if accountBIC.Credits.Value <= 0 {
		return 0, nil
	}
	decayedBIC, err := apiForSlot.ManaDecayProvider().ManaWithDecay(iotago.Mana(accountBIC.Credits.Value), accountBIC.Credits.UpdateTime, slot)
	if err != nil {
		return 0, ierrors.Wrapf(err, "failed to apply mana decay for account %s", accountID)
	}

	return decayedBIC, nil
}
