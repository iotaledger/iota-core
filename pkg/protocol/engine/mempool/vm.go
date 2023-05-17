package mempool

import (
	"context"
)

type VM func(ctx context.Context, stateTransition Transaction, inputs []State) (outputs []State, err error)
