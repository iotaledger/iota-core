package mempoolv1

import (
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledger"
	iotago "github.com/iotaledger/iota.go/v4"
)

type StateWithMetadata struct {
	id    iotago.OutputID
	state ledger.State

	*StateInclusion
	*SpentState
}

func NewStateWithMetadata(state ledger.State, optSource ...*TransactionMetadata) *StateWithMetadata {
	return &StateWithMetadata{
		id:    state.ID(),
		state: state,

		StateInclusion: NewStateInclusion().dependsOnCreatingTransaction(lo.First(optSource)),
		SpentState:     NewSpentState(),
	}
}

func (s *StateWithMetadata) ID() iotago.OutputID {
	return s.id
}

func (s *StateWithMetadata) State() ledger.State {
	return s.state
}
