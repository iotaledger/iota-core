package mempooltests

import (
	"context"

	"github.com/iotaledger/hive.go/ierrors"
	ledgertests "github.com/iotaledger/iota-core/pkg/protocol/engine/ledger/tests"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
)

func VM(_ context.Context, inputTransaction mempool.SignedTransaction, _ []mempool.OutputState, _ mempool.ContextState) (outputs []mempool.OutputState, err error) {
	transaction, ok := inputTransaction.(*Transaction)
	if !ok {
		return nil, ierrors.New("invalid transaction type in MockedVM")
	}

	if transaction.invalidTransaction {
		return nil, ierrors.New("invalid transaction")
	}

	for i := uint16(0); i < transaction.outputCount; i++ {
		id, err := transaction.ID()
		if err != nil {
			return nil, err
		}

		outputs = append(outputs, ledgertests.NewMockedState(id, i))
	}

	return outputs, nil
}
