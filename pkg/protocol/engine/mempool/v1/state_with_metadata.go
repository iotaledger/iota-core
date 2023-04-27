package mempoolv1

import (
	"iota-core/pkg/protocol/engine/ledger"

	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
)

type StateWithMetadata struct {
	id       iotago.OutputID
	source   *TransactionWithMetadata
	spenders *advancedset.AdvancedSet[*TransactionWithMetadata]
	state    ledger.State
}

func NewStateWithMetadata(state ledger.State, optSource ...*TransactionWithMetadata) *StateWithMetadata {
	return &StateWithMetadata{
		id:       state.ID(),
		source:   lo.First(optSource),
		spenders: advancedset.New[*TransactionWithMetadata](),
		state:    state,
	}
}

func (s *StateWithMetadata) OnSpent(func(spender *TransactionWithMetadata)) {
	// TODO: implement me
}

func (s *StateWithMetadata) ID() iotago.OutputID {
	return s.id
}

func (s *StateWithMetadata) State() ledger.State {
	return s.state
}

func (s *StateWithMetadata) IsSpent() bool {
	return s.spenders.Size() > 0
}
