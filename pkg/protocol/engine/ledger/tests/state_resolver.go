package ledgertests

import (
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/iota-core/pkg/core/promise"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	iotago "github.com/iotaledger/iota.go/v4"
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

func (s *MockStateResolver) ResolveOutputState(reference iotago.Input) *promise.Promise[mempool.State] {
	if reference.Type() == iotago.InputUTXO {
		output, exists := s.statesByID.Get(reference.ReferencedStateID())
		if !exists {
			return promise.New[mempool.State]().Reject(ierrors.Errorf("output %s not found: %w", reference.ReferencedStateID().ToHex(), mempool.ErrStateNotFound))
		}

		return promise.New[mempool.State]().Resolve(output)
	} else if reference.Type() == iotago.InputCommitment {
		//nolint:forcetypeassert
		output := &iotago.Commitment{
			ProtocolVersion:      0,
			Slot:                 0,
			PreviousCommitmentID: reference.(*iotago.CommitmentInput).CommitmentID,
			RootsID:              iotago.Identifier{},
			CumulativeWeight:     0,
			ReferenceManaCost:    0,
		}

		return promise.New[mempool.State]().Resolve(output)
	}

	return promise.New[mempool.State]().Reject(ierrors.Errorf("state not found"))
}

func (s *MockStateResolver) Cleanup() {
	s.statesByID.Clear()
}
