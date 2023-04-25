package mempool

import (
	"github.com/iotaledger/hive.go/ds/advancedset"
	iotago "github.com/iotaledger/iota.go/v4"
)

type TransactionWithMetadata interface {
	ID() iotago.TransactionID

	Transaction() Transaction

	Outputs() *advancedset.AdvancedSet[StateWithMetadata]

	IsSolid() bool

	IsExecuted() bool

	IsBooked() bool

	IsInvalid() bool

	IsEvicted() bool

	HookSolid(func())

	HookExecuted(func())

	HookBooked(func())

	HookInvalid(func(error))

	HookEvicted(func())
}
