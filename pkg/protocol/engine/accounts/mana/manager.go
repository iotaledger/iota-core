package mana

import (
	"github.com/zyedidia/generic/cache"
	"golang.org/x/xerrors"

	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger"
	iotago "github.com/iotaledger/iota.go/v4"
)

// For stored Mana added to account, or stored/potential Mana spent, we will update on commitment
// For potential Mana updates and decay, we update on demand if the Mana vector is accessed (by the scheduler)
type Manager struct {
	decayProvider *iotago.DecayProvider

	manaVectorCache *cache.Cache[iotago.AccountID, *accounts.Mana]

	accountOutputResolveFunc func(iotago.AccountID, iotago.SlotIndex) (*utxoledger.Output, error)

	mutex syncutils.Mutex

	module.Module
}

func NewManager(decayProvider *iotago.DecayProvider, accountOutputResolveFunc func(iotago.AccountID, iotago.SlotIndex) (*utxoledger.Output, error)) *Manager {
	return &Manager{
		decayProvider:            decayProvider,
		accountOutputResolveFunc: accountOutputResolveFunc,
		manaVectorCache:          cache.New[iotago.AccountID, *accounts.Mana](10000),
	}
}

func (m *Manager) GetManaOnAccount(accountID iotago.AccountID, currentSlot iotago.SlotIndex) (uint64, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	mana, exists := m.manaVectorCache.Get(accountID)
	if !exists {
		output, err := m.accountOutputResolveFunc(accountID, currentSlot)
		if err != nil {
			return 0, xerrors.Errorf("failed to resolve AccountOutput for %s in  slot %s: %w", accountID, currentSlot, err)
		}

		mana = accounts.NewMana(output.StoredMana(), output.Deposit(), output.CreationTime())

		m.manaVectorCache.Put(accountID, mana)
	}

	if currentSlot == mana.UpdateTime() {
		return mana.Value(), nil
	}

	// apply decay to stored Mana and potential that was added on last update
	updatedValue := m.decayProvider.StoredManaWithDecay(mana.Value(), currentSlot-mana.UpdateTime())
	// get newly generated potential since last update and apply decay
	updatedValue += m.decayProvider.PotentialManaWithDecay(mana.Deposit(), currentSlot-mana.UpdateTime())
	mana.UpdateValue(updatedValue, currentSlot)

	return mana.Value(), nil
}

func (m *Manager) CommitSlot(slotIndex iotago.SlotIndex, destroyedAccounts *advancedset.AdvancedSet[iotago.AccountID], accountOutputs map[iotago.AccountID]*utxoledger.Output) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	destroyedAccounts.Range(func(accountID iotago.AccountID) {
		m.manaVectorCache.Remove(accountID)
	})

	for accountID, output := range accountOutputs {
		mana, exists := m.manaVectorCache.Get(accountID)
		if exists {
			mana.Update(output.Output().StoredMana(), output.Output().Deposit(), slotIndex)
		}
	}
}
