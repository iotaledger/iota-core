package mempool

import (
	"context"
)

type VM func(ctx context.Context, stateTransition SignedTransaction, inputs []OutputState, timeReference ContextState) (outputs []OutputState, err error)
