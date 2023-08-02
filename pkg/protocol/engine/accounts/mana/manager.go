package mana

import (
	"github.com/zyedidia/generic/cache"

	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger"
	iotago "github.com/iotaledger/iota.go/v4"
)

// Manager is used to access stored and potential mana of an account in order.
// For stored Mana added to account, or stored/potential Mana spent, we will update on commitment.
// For potential Mana updates and decay, we update on demand if the Mana vector is accessed (by the scheduler).
type Manager struct {
	manaDecayProvider *iotago.ManaDecayProvider

	rentStructure *iotago.RentStructure

	manaVectorCache *cache.Cache[iotago.AccountID, *accounts.Mana]

	accountOutputResolveFunc func(iotago.AccountID, iotago.SlotIndex) (*utxoledger.Output, error)

	mutex syncutils.Mutex

	module.Module
}

func NewManager(manaDecayProvider *iotago.ManaDecayProvider, rentStructure *iotago.RentStructure, accountOutputResolveFunc func(iotago.AccountID, iotago.SlotIndex) (*utxoledger.Output, error)) *Manager {
	return &Manager{
		manaDecayProvider:        manaDecayProvider,
		rentStructure:            rentStructure,
		accountOutputResolveFunc: accountOutputResolveFunc,
		manaVectorCache:          cache.New[iotago.AccountID, *accounts.Mana](10000),
	}
}

func (m *Manager) GetManaOnAccount(accountID iotago.AccountID, currentSlot iotago.SlotIndex) (iotago.Mana, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	mana, exists := m.manaVectorCache.Get(accountID)
	if !exists {
		output, err := m.accountOutputResolveFunc(accountID, currentSlot)
		if err != nil {
			return 0, ierrors.Errorf("failed to resolve AccountOutput for %s in slot %s: %w", accountID, currentSlot, err)
		}
		minDeposit := m.rentStructure.MinDeposit(output.Output())
		if output.BaseTokenAmount() <= minDeposit {
			mana = accounts.NewMana(output.StoredMana(), 0, output.CreationTime())
		} else {
			mana = accounts.NewMana(output.StoredMana(), output.BaseTokenAmount()-minDeposit, output.CreationTime())
		}

		m.manaVectorCache.Put(accountID, mana)
	}

	if currentSlot == mana.UpdateTime() {
		return mana.Value(), nil
	}

	// apply decay to stored Mana and potential that was added on last update
	manaStored, err := m.manaDecayProvider.StoredManaWithDecay(mana.Value(), mana.UpdateTime(), currentSlot)
	if err != nil {
		return 0, err
	}
	updatedValue := manaStored

	// get newly generated potential since last update and apply decay
	manaPotential, err := m.manaDecayProvider.PotentialManaWithDecay(mana.ExcessBaseTokens(), mana.UpdateTime(), currentSlot)
	if err != nil {
		return 0, err
	}
	updatedValue += manaPotential

	mana.UpdateValue(updatedValue, currentSlot)

	return mana.Value(), nil
}

func (m *Manager) ApplyDiff(slotIndex iotago.SlotIndex, destroyedAccounts ds.Set[iotago.AccountID], accountOutputs map[iotago.AccountID]*utxoledger.Output) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	destroyedAccounts.Range(func(accountID iotago.AccountID) {
		m.manaVectorCache.Remove(accountID)
	})

	for accountID, output := range accountOutputs {
		mana, exists := m.manaVectorCache.Get(accountID)
		if exists {
			minDeposit := m.rentStructure.MinDeposit(output.Output())
			if output.BaseTokenAmount() <= minDeposit {
				mana.Update(output.StoredMana(), 0, slotIndex)
			} else {
				mana.Update(output.StoredMana(), output.BaseTokenAmount()-minDeposit, slotIndex)
			}
		}
	}
}
