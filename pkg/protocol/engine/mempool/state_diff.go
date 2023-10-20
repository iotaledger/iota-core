package mempool

import (
	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/ds/orderedmap"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	iotago "github.com/iotaledger/iota.go/v4"
)

// StateDiff is a collection of changes that happened in a certain slot and that can be applied to the ledger state.
type StateDiff interface {
	// Slot returns the slot index of the state diff.
	Slot() iotago.SlotIndex

	// DestroyedStates returns a compacted list of all the states that were destroyed in the slot.
	DestroyedStates() *shrinkingmap.ShrinkingMap[StateID, StateMetadata]

	// CreatedStates returns a compacted list of all the states that were created in the slot.
	CreatedStates() *shrinkingmap.ShrinkingMap[StateID, StateMetadata]

	// ExecutedTransactions returns an un-compacted list of all the transactions that were executed in the slot.
	ExecutedTransactions() *orderedmap.OrderedMap[iotago.TransactionID, TransactionMetadata]

	// Mutations returns an authenticated data structure that allows to commit to the applied mutations.
	Mutations() ads.Set[iotago.Identifier, iotago.TransactionID]

	// Reset resets the component to a clean state as if it was created at the last commitment.
	Reset() error
}
