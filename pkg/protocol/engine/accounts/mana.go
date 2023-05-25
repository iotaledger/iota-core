package accounts

import iotago "github.com/iotaledger/iota.go/v4"

// ManaHoldings is the stored and potential mana value of an account collected on the UTXO layer - used by the Scheduler.
type ManaHoldings struct {
	StoredValue    int64            `serix:"0"`
	PotentialValue int64            `serix:"1"`
	UpdateTime     iotago.SlotIndex `serix:"2"`
}

