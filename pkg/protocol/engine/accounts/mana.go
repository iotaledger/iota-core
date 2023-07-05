package accounts

import (
	"github.com/iotaledger/hive.go/runtime/syncutils"
	iotago "github.com/iotaledger/iota.go/v4"
)

// Mana is the stored and potential mana value of an account collected on the UTXO layer - used by the Scheduler.
type Mana struct {
	value      iotago.Mana      `serix:"0"`
	deposit    iotago.BaseToken `serix:"1"`
	updateTime iotago.SlotIndex `serix:"2"`

	mutex syncutils.RWMutex
}

func NewMana(value iotago.Mana, deposit iotago.BaseToken, updateTime iotago.SlotIndex) *Mana {
	return &Mana{
		value:      value,
		deposit:    deposit,
		updateTime: updateTime,
	}
}

func (m *Mana) Update(value iotago.Mana, deposit iotago.BaseToken, updateTime iotago.SlotIndex) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.value = value
	m.deposit = deposit
	m.updateTime = updateTime
}

func (m *Mana) UpdateValue(value iotago.Mana, updateTime iotago.SlotIndex) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.value = value
	m.updateTime = updateTime
}

func (m *Mana) Value() iotago.Mana {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	return m.value
}

func (m *Mana) Deposit() iotago.BaseToken {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	return m.deposit
}

func (m *Mana) UpdateTime() iotago.SlotIndex {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	return m.updateTime
}
