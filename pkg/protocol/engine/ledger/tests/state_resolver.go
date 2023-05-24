package ledgertests

import (
	"golang.org/x/xerrors"

	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/iota-core/pkg/core/promise"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	iotago "github.com/iotaledger/iota.go/v4"
)

type MockStateResolver struct {
	statesByID *shrinkingmap.ShrinkingMap[iotago.OutputID, mempool.State]
}

func New(initialStates ...mempool.State) *MockStateResolver {
	stateResolver := &MockStateResolver{
		statesByID: shrinkingmap.New[iotago.OutputID, mempool.State](),
	}
	for _, initialState := range initialStates {
		stateResolver.statesByID.Set(initialState.OutputID(), initialState)
	}

	return stateResolver
}

func (s *MockStateResolver) AddState(state mempool.State) {
	s.statesByID.Set(state.OutputID(), state)
}

func (s *MockStateResolver) DestroyState(stateID iotago.OutputID) {
	s.statesByID.Delete(stateID)
}

func (s *MockStateResolver) ResolveState(id iotago.OutputID) *promise.Promise[mempool.State] {
	output, exists := s.statesByID.Get(id)
	if !exists {
		return promise.New[mempool.State]().Reject(xerrors.Errorf("output %s not found: %w", id.ToHex(), mempool.ErrStateNotFound))
	}

	return promise.New[mempool.State]().Resolve(output)
}

func (s *MockStateResolver) Cleanup() {
	s.statesByID.Clear()
}
