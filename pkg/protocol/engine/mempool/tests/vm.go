package mempooltests

import (
	"context"

	"github.com/iotaledger/hive.go/ierrors"
	ledgertests "github.com/iotaledger/iota-core/pkg/protocol/engine/ledger/tests"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

func VM(_ context.Context, inputTransaction mempool.Transaction, _ []mempool.State) (outputs []mempool.State, err error) {
	transaction, ok := inputTransaction.(*Transaction)
	if !ok {
		return nil, ierrors.New("invalid transaction type in MockedVM")
	}

	if transaction.invalidTransaction {
		return nil, ierrors.New("invalid transaction")
	}

	for i := uint16(0); i < transaction.outputCount; i++ {
		id, err := transaction.ID(tpkg.TestAPI)
		if err != nil {
			return nil, err
		}

		outputs = append(outputs, ledgertests.NewMockedState(id, i))
	}

	return outputs, nil
}
