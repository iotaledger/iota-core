package mempoolv1

import (
	"iota-core/pkg/protocol/engine/vm"

	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
)

type StateMetadata struct {
	ID       iotago.OutputID
	Source   *TransactionMetadata
	Spenders *advancedset.AdvancedSet[*TransactionMetadata]
	state    vm.State
}

func NewStateMetadata(state vm.State, optSource ...*TransactionMetadata) *StateMetadata {
	return &StateMetadata{
		ID:       state.ID(),
		Source:   lo.First(optSource),
		Spenders: advancedset.New[*TransactionMetadata](),
		state:    state,
	}
}

func (o *StateMetadata) State() vm.State {
	return o.state
}

func (o *StateMetadata) IsSpent() bool {
	return o.Spenders.Size() > 0
}

func (o *StateMetadata) IsSolid() bool {
	return o.Source != nil
}
