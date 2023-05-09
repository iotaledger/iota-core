package mempoolv1

import (
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledger"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	iotago "github.com/iotaledger/iota.go/v4"
)

type StateWithMetadata struct {
	id    iotago.OutputID
	state ledger.State

	inclusionState *InclusionState
	spentState     *SpentState
}

func NewStateWithMetadata(state ledger.State, optSource ...*TransactionWithMetadata) *StateWithMetadata {
	s := &StateWithMetadata{
		id:    state.ID(),
		state: state,

		inclusionState: NewInclusionState(),
		spentState:     NewSpentState(),
	}

	if source := lo.First(optSource); source != nil {
		source.inclusion.OnAccepted(s.inclusionState.setAccepted)
		source.inclusion.OnRejected(s.inclusionState.setRejected)
		source.inclusion.OnCommitted(s.inclusionState.setCommitted)
		source.inclusion.OnOrphaned(s.inclusionState.setOrphaned)
	}

	return s
}

func (s *StateWithMetadata) ID() iotago.OutputID {
	return s.id
}

func (s *StateWithMetadata) State() ledger.State {
	return s.state
}

func (s *StateWithMetadata) InclusionState() mempool.InclusionState {
	return s.inclusionState
}

func (s *StateWithMetadata) SpentState() mempool.SpentState {
	return s.spentState
}
