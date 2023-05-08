package mempool

import (
	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/ds/orderedmap"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	iotago "github.com/iotaledger/iota.go/v4"
)

type StateDiff interface {
	// Index returns a SlotIndex for which the StateDiff is constructed.
	Index() iotago.SlotIndex

	// SpentOutputs returns a collection of compacted spent outputs.
	SpentOutputs() *shrinkingmap.ShrinkingMap[iotago.OutputID, StateWithMetadata]

	// CreatedOutputs returns a collection of compacted created outputs.
	CreatedOutputs() *shrinkingmap.ShrinkingMap[iotago.OutputID, StateWithMetadata]

	// ExecutedTransactions returns an ordered map of included transactions (from which we can retrieve un-compacted
	// spent and created states).
	ExecutedTransactions() *orderedmap.OrderedMap[iotago.TransactionID, TransactionWithMetadata]

	Mutations() *ads.Set[iotago.TransactionID, *iotago.TransactionID]
}
