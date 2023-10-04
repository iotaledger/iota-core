package ledgertests

import (
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/iota-core/pkg/core/promise"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
)

type MockStateResolver struct {
	statesByID *shrinkingmap.ShrinkingMap[mempool.StateID, mempool.State]
}

func New(initialStates ...mempool.State) *MockStateResolver {
	stateResolver := &MockStateResolver{
		statesByID: shrinkingmap.New[mempool.StateID, mempool.State](),
	}
	for _, initialState := range initialStates {
		stateResolver.statesByID.Set(initialState.StateID(), initialState)
	}

	return stateResolver
}

func (s *MockStateResolver) AddOutputState(state mempool.State) {
	s.statesByID.Set(state.StateID(), state)
}

func (s *MockStateResolver) DestroyOutputState(stateID mempool.StateID) {
	s.statesByID.Delete(stateID)
}

func (s *MockStateResolver) ResolveOutputState(reference mempool.StateReference) *promise.Promise[mempool.State] {
	output, exists := s.statesByID.Get(reference.ReferencedStateID())
	if !exists {
		return promise.New[mempool.State]().Reject(ierrors.Errorf("output %s not found: %w", reference.ReferencedStateID().ToHex(), mempool.ErrStateNotFound))
	}

	return promise.New[mempool.State]().Resolve(output)
}

func (s *MockStateResolver) Cleanup() {
	s.statesByID.Clear()
}
