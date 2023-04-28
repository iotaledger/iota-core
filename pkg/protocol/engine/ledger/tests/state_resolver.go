package ledgertests

import (
	"golang.org/x/xerrors"
	"iota-core/pkg/core/promise"
	"iota-core/pkg/protocol/engine/ledger"

	"github.com/iotaledger/hive.go/lo"
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
