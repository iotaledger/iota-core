package mempool

import (
	"context"

	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledger"
)

type VM func(ctx context.Context, stateTransition Transaction, inputs []ledger.State) (outputs []ledger.State, err error)
