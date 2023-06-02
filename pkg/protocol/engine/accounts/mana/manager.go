package mana

import (
	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/pkg/errors"
)

// For stored Mana added to account, or stored/potential Mana spent, we will update on commitment
// For potential Mana updates and decay, we update on demand if the Mana vector is accessed (by the scheduler)
type Manager struct {
	protocolParams iotago.ProtocolParameters

	manaVector *shrinkingmap.ShrinkingMap[iotago.AccountID, *accounts.Mana]

	// TODO: properly lock across methods
	mutex syncutils.RWMutex

	module.Module
}

func (m *Manager) GetManaOnAccount(accountID iotago.AccountID, currentSlot iotago.SlotIndex) (uint64, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	mana, exists := m.manaVector.Get(accountID)
	if !exists {
		return 0, errors.Errorf("mana for accountID %s does not exist in this slot", accountID)
	}

	if currentSlot == mana.UpdateTime() {
		return mana.Value(), nil
	}

	// apply decay to stored Mana and potential that was added on last update
	updatedValue := m.protocolParams.StoredManaWithDecay(mana.Value(), currentSlot-mana.UpdateTime())
	// get newly generated potential since last update and apply decay
	updatedValue += m.protocolParams.PotentialManaWithDecay(mana.Deposit(), currentSlot-mana.UpdateTime())
	mana.UpdateValue(updatedValue, currentSlot)

	return mana.Value(), nil
}

func (m *Manager) CommitSlot(slotIndex iotago.SlotIndex, destroyedAccounts *advancedset.AdvancedSet[iotago.AccountID], accountOutputs map[iotago.AccountID]*iotago.AccountOutput) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	destroyedAccounts.Range(func(accountID iotago.AccountID) {
		m.manaVector.Delete(accountID)
	})

	for accountID, accountOutput := range accountOutputs {
		mana, _ := m.manaVector.GetOrCreate(accountID, func() *accounts.Mana {
			return accounts.NewMana(accountOutput.StoredMana(), accountOutput.Deposit(), slotIndex)
		})
		mana.Update(accountOutput.StoredMana(), accountOutput.Deposit(), slotIndex)
	}
}
