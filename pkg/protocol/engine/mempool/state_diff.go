package mempool

import (
	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/ds/orderedmap"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	iotago "github.com/iotaledger/iota.go/v4"
)

// StateDiff is a collection of changes that happened in a certain slot and that can be applied to the ledger state.
type StateDiff interface {
	// Index returns the slot index of the state diff.
	Index() iotago.SlotIndex

	// DestroyedStates returns a compacted list of all the states that were destroyed in the slot.
	DestroyedStates() *shrinkingmap.ShrinkingMap[iotago.OutputID, StateWithMetadata]

	// CreatedStates returns a compacted list of all the states that were created in the slot.
	CreatedStates() *shrinkingmap.ShrinkingMap[iotago.OutputID, StateWithMetadata]

	// ExecutedTransactions returns an un-compacted list of all the transactions that were executed in the slot.
	ExecutedTransactions() *orderedmap.OrderedMap[iotago.TransactionID, TransactionWithMetadata]

	// Mutations returns an authenticated data structure that allows to commit to the applied mutations.
	Mutations() *ads.Set[iotago.TransactionID, *iotago.TransactionID]
}
