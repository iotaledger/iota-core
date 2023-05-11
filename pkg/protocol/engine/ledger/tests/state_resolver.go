package ledgertests

import (
	"golang.org/x/xerrors"

	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/iota-core/pkg/core/promise"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledger"
	iotago "github.com/iotaledger/iota.go/v4"
)

type StateResolver struct {
	statesByID *shrinkingmap.ShrinkingMap[iotago.OutputID, ledger.State]
}

func New(initialStates ...ledger.State) *StateResolver {
	stateResolver := &StateResolver{
		statesByID: shrinkingmap.New[iotago.OutputID, ledger.State](),
	}
	for _, initialState := range initialStates {
		stateResolver.statesByID.Set(initialState.ID(), initialState)
	}

	return stateResolver
}

func (s *StateResolver) AddState(state ledger.State) {
	s.statesByID.Set(state.ID(), state)
}

func (s *StateResolver) DestroyState(stateID iotago.OutputID) {
	s.statesByID.Delete(stateID)
}

func (s *StateResolver) ResolveState(id iotago.OutputID) *promise.Promise[ledger.State] {
	output, exists := s.statesByID.Get(id)
	if !exists {
		return promise.New[ledger.State]().Reject(xerrors.Errorf("output %s not found: %w", id.ToHex(), ledger.ErrStateNotFound))
	}

	return promise.New[ledger.State]().Resolve(output)
}

func (s *StateResolver) Cleanup() {
	s.statesByID.Clear()
}
