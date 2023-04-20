package mempool

import (
	"iota-core/pkg/types"

	"github.com/iotaledger/hive.go/ds/advancedset"
	iotago "github.com/iotaledger/iota.go/v4"
)

type OutputMetadata struct {
	ID       iotago.OutputID
	Source   *TransactionMetadata
	Spenders *advancedset.AdvancedSet[*TransactionMetadata]
	output   types.Output
}

func (o *OutputMetadata) Output() types.Output {
	return o.output
}

func (o *OutputMetadata) IsSpent() bool {
	return o.Spenders.Size() > 0
}

func (o *OutputMetadata) IsSolid() bool {
	return o.Source != nil
}
