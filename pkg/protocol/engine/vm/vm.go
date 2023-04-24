package vm

import "context"

type VM func(stateTransition StateTransition, inputs []State, ctx context.Context) (outputs []State, err error)
