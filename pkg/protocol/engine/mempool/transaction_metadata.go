package mempool

import (
	"github.com/iotaledger/hive.go/ds/advancedset"
	iotago "github.com/iotaledger/iota.go/v4"
)

type TransactionWithMetadata interface {
	ID() iotago.TransactionID

	Transaction() Transaction

	IsStored() bool

	IsSolid() bool

	IsBooked() bool

	IsExecuted() bool

	Outputs() *advancedset.AdvancedSet[StateWithMetadata]
}
