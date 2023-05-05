package mempool

import (
	"github.com/iotaledger/hive.go/ds/advancedset"
	iotago "github.com/iotaledger/iota.go/v4"
)

type TransactionWithMetadata interface {
	ID() iotago.TransactionID

	Transaction() Transaction

	Outputs() *advancedset.AdvancedSet[StateWithMetadata]

	SetCommitted()

	IsSolid() bool

	IsExecuted() bool

	IsBooked() bool

	IsInvalid() bool

	IsEvicted() bool

	IsAccepted() bool

	OnSolid(func())

	OnExecuted(func())

	OnBooked(func())

	OnInvalid(func(error))

	OnEvicted(func())
}
