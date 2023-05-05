package mempool

import (
	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/ds/advancedset"
	iotago "github.com/iotaledger/iota.go/v4"
)

type StateDiff struct {
	Index         iotago.SlotIndex
	StateMutation *ads.Set[iotago.TransactionID, *iotago.TransactionID]
	Transactions  *advancedset.AdvancedSet[TransactionWithMetadata]
}
