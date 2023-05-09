package ledgertests

import (
	"golang.org/x/xerrors"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/core/promise"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledger"
	iotago "github.com/iotaledger/iota.go/v4"
)

type StateResolver struct {
	statesByID map[iotago.OutputID]ledger.State
}

func New(initialStates ...ledger.State) *StateResolver {
	return &StateResolver{
		statesByID: lo.KeyBy(initialStates, ledger.State.ID),
	}
}

func (s *StateResolver) AddState(state ledger.State) {
	s.statesByID[state.ID()] = state
}

func (s *StateResolver) ResolveState(id iotago.OutputID) *promise.Promise[ledger.State] {
	output, exists := s.statesByID[id]
	if !exists {
		return promise.New[ledger.State]().Reject(xerrors.Errorf("output %s not found: %w", id.ToHex(), ledger.ErrStateNotFound))
	}

	return promise.New[ledger.State]().Resolve(output)
}

func (s *StateResolver) Cleanup() {
	s.statesByID = make(map[iotago.OutputID]ledger.State)
}
