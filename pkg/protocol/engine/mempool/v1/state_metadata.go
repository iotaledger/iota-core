package mempoolv1

import (
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledger"
	iotago "github.com/iotaledger/iota.go/v4"
)

type StateMetadata struct {
	id    iotago.OutputID
	state ledger.State

	*StateInclusion
	*StateLifecycle
}

func NewStateMetadata(state ledger.State, optSource ...*TransactionMetadata) *StateMetadata {
	return &StateMetadata{
		id:    state.ID(),
		state: state,

		StateInclusion: NewStateInclusion().dependsOnCreatingTransaction(lo.First(optSource)),
		StateLifecycle: NewStateLifecycle(),
	}
}

func (s *StateMetadata) ID() iotago.OutputID {
	return s.id
}

func (s *StateMetadata) State() ledger.State {
	return s.state
}
