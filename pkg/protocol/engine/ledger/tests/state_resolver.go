package ledgertests

import (
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/iota-core/pkg/core/promise"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	iotago "github.com/iotaledger/iota.go/v4"
)

type MockStateResolver struct {
	statesByID *shrinkingmap.ShrinkingMap[iotago.OutputID, mempool.OutputState]
}

func New(initialStates ...mempool.OutputState) *MockStateResolver {
	stateResolver := &MockStateResolver{
		statesByID: shrinkingmap.New[iotago.OutputID, mempool.OutputState](),
	}
	for _, initialState := range initialStates {
		stateResolver.statesByID.Set(initialState.OutputID(), initialState)
	}

	return stateResolver
}

func (s *MockStateResolver) AddOutputState(state mempool.OutputState) {
	s.statesByID.Set(state.OutputID(), state)
}

func (s *MockStateResolver) DestroyOutputState(stateID iotago.OutputID) {
	s.statesByID.Delete(stateID)
}

func (s *MockStateResolver) ResolveOutputState(outputID iotago.OutputID) *promise.Promise[mempool.State] {
	output, exists := s.statesByID.Get(outputID)
	if !exists {
		return promise.New[mempool.State]().Reject(ierrors.Errorf("output %s not found: %w", outputID.ToHex(), mempool.ErrStateNotFound))
	}

	return promise.New[mempool.State]().Resolve(output)
}

func (s *MockStateResolver) Cleanup() {
	s.statesByID.Clear()
}
