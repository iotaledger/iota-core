package mempool

import (
	"context"
)

type VM func(ctx context.Context, stateTransition Transaction, inputs []OutputState) (outputs []OutputState, err error)
