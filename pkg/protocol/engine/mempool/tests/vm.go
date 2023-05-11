package mempooltests

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledger"
	ledgertests "github.com/iotaledger/iota-core/pkg/protocol/engine/ledger/tests"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
)

func VM(_ context.Context, inputTransaction mempool.Transaction, _ []ledger.State) (outputs []ledger.State, err error) {
	transaction, ok := inputTransaction.(*Transaction)
	if !ok {
		return nil, xerrors.Errorf("invalid transaction type in MockedVM")
	}

	for i := uint16(0); i < transaction.outputCount; i++ {
		id, err := transaction.ID()
		if err != nil {
			return nil, err
		}

		outputs = append(outputs, ledgertests.NewState(id, i))
	}

	return outputs, nil
}
