package mempool

import (
	"context"
)

type VM func(ctx context.Context, stateTransition Transaction, inputs []OutputState, timeReference ContextState) (outputs []OutputState, err error)
