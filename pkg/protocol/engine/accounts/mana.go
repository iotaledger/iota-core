package accounts

import iotago "github.com/iotaledger/iota.go/v4"

// Mana is the stored and potential mana value of an account collected on the UTXO layer - used by the Scheduler.
type Mana struct {
	StoredMana uint64           `serix:"0"`
	Deposit    uint64           `serix:"1"`
	Value      uint64           `serix:"2"`
	UpdateTime iotago.SlotIndex `serix:"3"`
}
