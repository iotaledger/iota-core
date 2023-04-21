package mempoolv1

import (
	iotago2 "iota-core/pkg/iotago"

	"github.com/iotaledger/hive.go/ds/advancedset"
	iotago "github.com/iotaledger/iota.go/v4"
)

type OutputMetadata struct {
	ID       iotago.OutputID
	Source   *TransactionMetadata
	Spenders *advancedset.AdvancedSet[*TransactionMetadata]
	output   iotago2.Output
}

func (o *OutputMetadata) Output() iotago2.Output {
	return o.output
}

func (o *OutputMetadata) IsSpent() bool {
	return o.Spenders.Size() > 0
}

func (o *OutputMetadata) IsSolid() bool {
	return o.Source != nil
}
