package mempool

import (
	"context"

	"iota-core/pkg/protocol/engine/ledger"
)

type VM func(stateTransition Transaction, inputs []ledger.State, ctx context.Context) (outputs []ledger.State, err error)
