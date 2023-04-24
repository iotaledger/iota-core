package mempooltests

import (
	"context"

	"golang.org/x/xerrors"
	"iota-core/pkg/protocol/engine/vm"
	vmtests "iota-core/pkg/protocol/engine/vm/tests"
)

func VM(inputTransaction vm.StateTransition, inputs []vm.State, ctx context.Context) (outputs []vm.State, err error) {
	transaction, ok := inputTransaction.(*Transaction)
	if !ok {
		return nil, xerrors.Errorf("invalid transaction type in MockedVM")
	}

	for i := uint16(0); i < transaction.outputCount; i++ {
		id, err := transaction.ID()
		if err != nil {
			return nil, err
		}

		outputs = append(outputs, vmtests.NewState(id, i))
	}

	return outputs, nil
}
