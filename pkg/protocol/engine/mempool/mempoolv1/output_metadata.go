package mempoolv1

import (
	"iota-core/pkg/protocol/engine/ledger"

	"github.com/iotaledger/hive.go/ds/advancedset"
	iotago "github.com/iotaledger/iota.go/v4"
)

type OutputMetadata struct {
	ID       iotago.OutputID
	Source   *TransactionMetadata
	Spenders *advancedset.AdvancedSet[*TransactionMetadata]
	output   ledger.Output
}

func (o *OutputMetadata) Output() ledger.Output {
	return o.output
}

func (o *OutputMetadata) IsSpent() bool {
	return o.Spenders.Size() > 0
}

func (o *OutputMetadata) IsSolid() bool {
	return o.Source != nil
}
