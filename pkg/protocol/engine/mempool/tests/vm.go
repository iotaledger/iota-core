package mempooltests

import (
	"context"

	"github.com/iotaledger/hive.go/ierrors"
	ledgertests "github.com/iotaledger/iota-core/pkg/protocol/engine/ledger/tests"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
)

type VM struct{}

func (V *VM) Inputs(transaction mempool.Transaction) ([]mempool.StateReference, error) {
	testTransaction, ok := transaction.(*Transaction)
	if !ok {
		return nil, ierrors.New("invalid transaction type in MockedVM")
	}

	return testTransaction.Inputs()
}

func (V *VM) ValidateSignatures(_ mempool.SignedTransaction, _ []mempool.State) (executionContext context.Context, err error) {
	return context.Background(), nil
}

func (V *VM) Execute(_ context.Context, transaction mempool.Transaction) (outputs []mempool.State, err error) {
	typedTransaction, ok := transaction.(*Transaction)
	if !ok {
		return nil, ierrors.New("invalid transaction type in MockedVM")
	}

	if typedTransaction.invalidTransaction {
		return nil, ierrors.New("invalid transaction")
	}

	for i := uint16(0); i < typedTransaction.outputCount; i++ {
		id, err := typedTransaction.ID()
		if err != nil {
			return nil, err
		}

		outputs = append(outputs, ledgertests.NewMockedState(id, i))
	}

	return outputs, nil
}
