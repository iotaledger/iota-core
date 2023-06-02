package accounts

import (
	"github.com/iotaledger/hive.go/runtime/syncutils"
	iotago "github.com/iotaledger/iota.go/v4"
)

// Mana is the stored and potential mana value of an account collected on the UTXO layer - used by the Scheduler.
type Mana struct {
	value      uint64           `serix:"0"`
	deposit    uint64           `serix:"1"`
	updateTime iotago.SlotIndex `serix:"2"`

	mutex syncutils.RWMutex
}

func NewMana(value uint64, deposit uint64, updateTime iotago.SlotIndex) *Mana {
	return &Mana{
		value:      value,
		deposit:    deposit,
		updateTime: updateTime,
	}
}

func (m *Mana) Update(value uint64, deposit uint64, updateTime iotago.SlotIndex) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.value = value
	m.deposit = deposit
	m.updateTime = updateTime
}

func (m *Mana) UpdateValue(value uint64, updateTime iotago.SlotIndex) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.value = value
	m.updateTime = updateTime
}

func (m *Mana) Value() uint64 {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	return m.value
}

func (m *Mana) Deposit() uint64 {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	return m.deposit
}

func (m *Mana) UpdateTime() iotago.SlotIndex {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	return m.updateTime
}
