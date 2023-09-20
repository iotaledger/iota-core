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
	apiProvider iotago.APIProvider

	manaVectorCache *cache.Cache[iotago.AccountID, *accounts.Mana]

	accountOutputResolveFunc func(iotago.AccountID, iotago.SlotIndex) (*utxoledger.Output, error)

	mutex syncutils.Mutex

	module.Module
}

func NewManager(apiProvider iotago.APIProvider, accountOutputResolveFunc func(iotago.AccountID, iotago.SlotIndex) (*utxoledger.Output, error)) *Manager {
	return &Manager{
		apiProvider:              apiProvider,
		accountOutputResolveFunc: accountOutputResolveFunc,
		manaVectorCache:          cache.New[iotago.AccountID, *accounts.Mana](10000),
	}
}

func (m *Manager) GetManaOnAccount(accountID iotago.AccountID, currentSlot iotago.SlotIndex) (iotago.Mana, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	mana, exists := m.manaVectorCache.Get(accountID)
	if !exists || mana.UpdateTime() > currentSlot {
		output, err := m.accountOutputResolveFunc(accountID, currentSlot)
		if err != nil {
			return 0, ierrors.Errorf("failed to resolve AccountOutput for %s in slot %s: %w", accountID, currentSlot, err)
		}
		minDeposit := m.apiProvider.CurrentAPI().ProtocolParameters().RentStructure().MinDeposit(output.Output())
		if output.BaseTokenAmount() <= minDeposit {
			mana = accounts.NewMana(output.StoredMana(), 0, output.CreationSlot())
		} else {
			mana = accounts.NewMana(output.StoredMana(), output.BaseTokenAmount()-minDeposit, output.CreationSlot())
		}

		if !exists {
			m.manaVectorCache.Put(accountID, mana)
		}
	}

	if currentSlot == mana.UpdateTime() {
		return mana.Value(), nil
	}

	manaDecayProvider := m.apiProvider.CurrentAPI().ManaDecayProvider()
	// apply decay to stored Mana and potential that was added on last update
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
			minDeposit := m.apiProvider.CurrentAPI().ProtocolParameters().RentStructure().MinDeposit(output.Output())
			if output.BaseTokenAmount() <= minDeposit {
				mana.Update(output.StoredMana(), 0, slotIndex)
			} else {
				mana.Update(output.StoredMana(), output.BaseTokenAmount()-minDeposit, slotIndex)
			}
		}
	}
}
