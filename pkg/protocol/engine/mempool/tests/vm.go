package mempooltests

import (
	"context"

	"github.com/iotaledger/hive.go/ierrors"
	ledgertests "github.com/iotaledger/iota-core/pkg/protocol/engine/ledger/tests"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	iotago "github.com/iotaledger/iota.go/v4"
)

type VM struct{}

func (V *VM) TransactionInputs(transaction mempool.Transaction) ([]iotago.Input, error) {
	return transaction.(*Transaction).Inputs()
}

func (V *VM) ValidateSignatures(signedTransaction mempool.SignedTransaction, resolvedInputs []mempool.State) (executionContext context.Context, err error) {
	return context.Background(), nil
}

func (V *VM) ExecuteTransaction(executionContext context.Context, transaction mempool.Transaction) (outputs []mempool.State, err error) {
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
