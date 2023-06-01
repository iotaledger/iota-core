package mana

import (
	"sync"

	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/runtime/module"
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
	mutex sync.RWMutex

	module.Module
}

func (m *Manager) GetManaOnAccount(accountID iotago.AccountID, currentSlot iotago.SlotIndex) (updatedValue uint64, err error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	oldMana, exists := m.manaVector.Get(accountID)
	if !exists {
		return 0, errors.Errorf("mana for accountID %s does not exist in this slot", accountID)
	}
	// apply decay to stored Mana and potential that was added on last update
	updatedValue += m.protocolParams.StoredManaWithDecay(oldMana.Value, currentSlot-oldMana.UpdateTime)
	// get newly generated potential since last update and apply decay
	updatedValue += m.protocolParams.PotentialManaWithDecay(oldMana.Deposit, currentSlot-oldMana.UpdateTime)

	oldMana.Value = updatedValue
	oldMana.UpdateTime = currentSlot
	return updatedValue, nil
}

func (m *Manager) UpdateManaOnAccount(accountID iotago.AccountID, newStoredMana uint64, newDeposit uint64, currentSlot iotago.SlotIndex) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.manaVector.Set(
		accountID,
		&accounts.Mana{
			StoredMana: newStoredMana,
			Deposit:    newDeposit,
			Value:      newStoredMana,
			UpdateTime: currentSlot,
		},
	)
}

func (m *Manager) CommitSlot() {
	// TODO: this method should take all committed changes to account ouputs from a slot and update the manavector for each.
}
