package mempoolv1

import (
	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/core/promise"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledger"
	iotago "github.com/iotaledger/iota.go/v4"
)

type StateWithMetadata struct {
	id                iotago.OutputID
	sourceTransaction *TransactionWithMetadata
	spenders          *advancedset.AdvancedSet[*TransactionWithMetadata]
	state             ledger.State

	accepted *promise.Event
}

func NewStateWithMetadata(state ledger.State, optSource ...*TransactionWithMetadata) *StateWithMetadata {
	return &StateWithMetadata{
		id:                state.ID(),
		sourceTransaction: lo.First(optSource),
		spenders:          advancedset.New[*TransactionWithMetadata](),
		state:             state,
		accepted:          promise.NewEvent(),
	}
}

func (s *StateWithMetadata) OnSpent(func(spender *TransactionWithMetadata)) {
	// TODO: implement me
}

func (s *StateWithMetadata) setAccepted() {
	s.accepted.Trigger()
}

func (s *StateWithMetadata) IsAccepted() bool {
	return s.accepted.WasTriggered()
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
