package mempool

import (
	"github.com/iotaledger/hive.go/ds/advancedset"
	iotago "github.com/iotaledger/iota.go/v4"
)

type TransactionMetadata interface {
	ID() iotago.TransactionID

	Transaction() Transaction

	Inputs() *advancedset.AdvancedSet[StateMetadata]

	Outputs() *advancedset.AdvancedSet[StateMetadata]

	TransactionInclusion

	TransactionLifecycle

	TransactionAttachments
}
