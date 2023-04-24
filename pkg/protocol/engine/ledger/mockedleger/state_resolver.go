package mockedleger

import (
	"golang.org/x/xerrors"
	"iota-core/pkg/protocol/engine/mempool/promise"
	"iota-core/pkg/protocol/engine/vm"

	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
)

type StateResolver struct {
	statesByID map[iotago.OutputID]vm.State
}

func NewStateResolver(initialStates ...vm.State) *StateResolver {
	return &StateResolver{
		statesByID: lo.KeyBy(initialStates, vm.State.ID),
	}
}

func (s *StateResolver) AddState(state vm.State) {
	s.statesByID[state.ID()] = state
}

func (s *StateResolver) ResolveState(id iotago.OutputID) *promise.Promise[vm.State] {
	output, exists := s.statesByID[id]
	if !exists {
		return promise.New[vm.State]().Reject(xerrors.Errorf("output %s not found", id))
	}

	return promise.New[vm.State]().Resolve(output)
}
